package main

import (
    "encoding/json"
    "fmt"
    "os"
    "net/http"
    "sync"
    "time"
    "context"
    "strings"

    extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
    "k8s.io/klog"

    "github.com/jetstack/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
    "github.com/jetstack/cert-manager/pkg/acme/webhook/cmd"
)

var GroupName = os.Getenv("GROUP_NAME")

func main() {
    if GroupName == "" {
        panic("GROUP_NAME must be specified")
    }

    cmd.RunWebhookServer(GroupName,
        &hostingdeDNSProviderSolver{},
    )
}

type Config struct {
    APIKey   string
    ZoneName string
    TTL      int
}

type hostingdeDNSProviderSolver struct {
    client      *kubernetes.Clientset
    config      *Config
    recordIDs   map[string]string
    recordIDsMu sync.Mutex
    httpClient  *http.Client
}

type hostingdeDNSProviderConfig struct {
    ZoneName  string  `json:"zoneName"`
    TTL       int     `json:"TTL"`
    SecretName string `json:"secretName"`
}

func (c *hostingdeDNSProviderSolver) Name() string {
    return "hostingde"
}

func UnFqdn(name string) string {
    n := len(name)
    if n != 0 && name[n-1] == '.' {
        return name[:n-1]
    }
    return name
}

func (c *hostingdeDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
    klog.V(6).Infof("call function Present: namespace=%s, zone=%s, fqdn=%s", ch.ResourceNamespace, ch.ResolvedZone, ch.ResolvedFQDN)

    var err error
    c.config, err = clientConfig(c, ch)
    if err != nil {
        return fmt.Errorf("unable to get secret `%s`; %v", ch.ResourceNamespace, err)
    }

    fqdn, value := ch.ResolvedFQDN, ch.Key

    // get the ZoneConfig for that domain
    zonesFind := ZoneConfigsFindRequest{
        Filter: Filter{
            Field: "zoneName",
            Value: c.config.ZoneName,
        },
        Limit: 1,
        Page:  1,
    }
    zonesFind.AuthToken = c.config.APIKey

    zoneConfig, err := c.getZone(zonesFind)
    if err != nil {
        return fmt.Errorf("hostingde: %w", err)
    }
    zoneConfig.Name = c.config.ZoneName

    rec := []DNSRecord{{
        Type:    "TXT",
        Name:    UnFqdn(fqdn),
        Content: value,
        TTL:     c.config.TTL,
    }}

    req := ZoneUpdateRequest{
        ZoneConfig:   *zoneConfig,
        RecordsToAdd: rec,
    }
    req.AuthToken = c.config.APIKey

    resp, err := c.updateZone(req)
    if err != nil {
        return fmt.Errorf("hostingde: %w", err)
    }

    for _, record := range resp.Response.Records {
        if record.Name == strings.ToLower(UnFqdn(fqdn)) && record.Content == fmt.Sprintf(`"%s"`, value) {
            c.recordIDsMu.Lock()
            c.recordIDs[fqdn] = record.ID
            c.recordIDsMu.Unlock()
        }
    }

    if c.recordIDs[fqdn] == "" {
        return fmt.Errorf("hostingde: error getting ID of just created record, for domain %s", ch.ResolvedFQDN)
    }

    klog.Infof("Presented txt record %v", ch.ResolvedFQDN)

    return nil
}

func (c *hostingdeDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
    fqdn, value := ch.ResolvedFQDN, ch.Key

    rec := []DNSRecord{{
        Type:    "TXT",
        Name:    UnFqdn(fqdn),
        Content: `"` + value + `"`,
    }}

    // get the ZoneConfig for that domain
    zonesFind := ZoneConfigsFindRequest{
        Filter: Filter{
            Field: "zoneName",
            Value: c.config.ZoneName,
        },
        Limit: 1,
        Page:  1,
    }
    zonesFind.AuthToken = c.config.APIKey

    zoneConfig, err := c.getZone(zonesFind)
    if err != nil {
        return fmt.Errorf("hostingde: %w", err)
    }
    zoneConfig.Name = c.config.ZoneName

    req := ZoneUpdateRequest{
        ZoneConfig:      *zoneConfig,
        RecordsToDelete: rec,
    }
    req.AuthToken = c.config.APIKey

    // Delete record ID from map
    c.recordIDsMu.Lock()
    delete(c.recordIDs, fqdn)
    c.recordIDsMu.Unlock()

    _, err = c.updateZone(req)
    if err != nil {
        return fmt.Errorf("hostingde: %w", err)
    }
    return nil
}

func (c *hostingdeDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
    cl, err := kubernetes.NewForConfig(kubeClientConfig)
    if err != nil {
        return err
    }

    c.client = cl
    c.recordIDs = map[string]string{}
    c.httpClient = &http.Client{
        Timeout: 30*time.Second,
    }

    return nil
}

func loadConfig(cfgJSON *extapi.JSON) (hostingdeDNSProviderConfig, error) {
    cfg := hostingdeDNSProviderConfig{}

    if cfgJSON == nil {
        return cfg, nil
    }
    if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
        return cfg, fmt.Errorf("error decoding solver config: %v", err)
    }

    return cfg, nil
}

func stringFromSecretData(secretData *map[string][]byte, key string) (string, error) {
    data, ok := (*secretData)[key]
    if !ok {
        return "", fmt.Errorf("key %q not found in secret data", key)
    }
    return string(data), nil
}

func clientConfig(c *hostingdeDNSProviderSolver, ch *v1alpha1.ChallengeRequest) (*Config, error) {
    var config Config

    cfg, err := loadConfig(ch.Config)
    if err != nil {
        return nil, err
    }
    config.ZoneName = cfg.ZoneName
    config.TTL = cfg.TTL

    secretName := cfg.SecretName
    sec, err := c.client.CoreV1().Secrets(ch.ResourceNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
    if err != nil {
        return nil, fmt.Errorf("unable to get secret `%s/%s`; %v", secretName, ch.ResourceNamespace, err)
    }

    apiKey, err := stringFromSecretData(&sec.Data, "api-key")
    if err != nil {
        return nil, fmt.Errorf("unable to get api-key from secret `%s/%s`; %v", secretName, ch.ResourceNamespace, err)
    }
    config.APIKey = apiKey

    return &config, nil
}
