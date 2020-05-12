# ACME webhook for hosting.de DNS API

This solver can be used when you want to use cert-manager with the hosting.de DNS API. API documentation is [here](https://www.hosting.de/api/)

## Requirements
-   [go](https://golang.org/) >= 1.14.0
-   [helm](https://helm.sh/) >= v3.0.0
-   [kubernetes](https://kubernetes.io/) >= v1.14.0
-   [cert-manager](https://cert-manager.io/) >= 0.15.0

## Installation

### cert-manager

Follow the [instructions](https://cert-manager.io/docs/installation/) using the cert-manager documentation to install it within your cluster.

### Webhook

```bash
helm install --namespace cert-manager cert-manager-webhook-hostingde deploy/cert-manager-webhook-hostingde
```
**Note**: The kubernetes resources used to install the Webhook should be deployed within the same namespace as the cert-manager.

To uninstall the webhook run
```bash
helm uninstall --namespace cert-manager cert-manager-webhook-hostingde
```


Alternatively, generate manifests from the template and apply them manually:
```bash
helm template --namespace cert-manager cert-manager-webhook-hostingde deploy/cert-manager-webhook-hostingde
```

## Issuer

Create a `ClusterIssuer` or `Issuer` resource as following:
```yaml
apiVersion: cert-manager.io/v1alpha3
kind: ClusterIssuer
metadata:
  name: letsencrypt-staging
spec:
  acme:
    # The ACME server URL
    server: https://acme-staging-v02.api.letsencrypt.org/directory

    # Email address used for ACME registration
    email: mail@example.com

    # Name of a secret used to store the ACME account private key
    privateKeySecretRef:
      name: letsencrypt-staging

    solvers:
      - dns01:
          webhook:
            groupName: acme.sealedplatform.com
            solverName: hostingde
            config:
              secretName: hostingde-secret
              zoneName: example.com
              TTL: 60
```

### Credentials
In order to access the hosting.de API, the webhook needs an API token.

If you choose another name for the secret than `hostingde-secret`, ensure you modify the value of `secretName` in the `[Cluster]Issuer`.

The secret for the example above will look like this:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: hostingde-secret
  namespace: cert-manager
type: Opaque
data:
  api-key: your-key-base64-encoded
```

### Create a certificate

Finally you can create certificates, for example:

```yaml
apiVersion: cert-manager.io/v1alpha3
kind: Certificate
metadata:
  name: example-cert
  namespace: default
spec:
  commonName: example.com
  dnsNames:
    - example.com
  issuerRef:
    name: letsencrypt-staging
    kind: ClusterIssuer
  secretName: example-cert
```

## Development

### Running the test suite

All DNS providers **must** run the DNS01 provider conformance testing suite,
else they will have undetermined behaviour when used with cert-manager.

**It is essential that you configure and run the test suite when creating a
DNS01 webhook.**

First, you need to have hosting.de account with access to the DNS control panel. You need to create an API token and have a registered DNS zone there.
Then you need to replace `zoneName` parameter at `testdata/hostingde/config.json` file with actual one.

You also must encode your api token into base64 and put it into the `testdata/hostingde/secret.yml` file:

In case there is a source IP restriction for the API key, you will also need to add your public IP address in the hosting.de control panel.
```bash
echo -n APIKEY | base64
```

You can then run the test suite with:

```bash
# first install necessary binaries (only required once)
scripts/fetch-test-binaries.sh

# then run the tests
TEST_ZONE_NAME=example.com. make verify
```

