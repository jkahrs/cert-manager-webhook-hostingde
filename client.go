package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
)

const defaultBaseURL = "https://secure.hosting.de/api/dns/v1/json"

// https://www.hosting.de/api/?json#list-zoneconfigs
func (d *hostingdeDNSProviderSolver) listZoneConfigs(findRequest ZoneConfigsFindRequest) (*ZoneConfigsFindResponse, error) {
	uri := defaultBaseURL + "/zoneConfigsFind"

	findResponse := &ZoneConfigsFindResponse{}

	_, err := d.post(uri, findRequest, findResponse)
	if err != nil {
		return nil, err
	}

	cacheableResponse := ZoneConfigsFindResponse{
		BaseResponse: BaseResponse{
			Errors:   findResponse.Errors,
			Warnings: findResponse.Warnings,
			Status:   findResponse.Status,
		},
		Response: findResponse.Response,
	}

	if len(findResponse.Response.Data) == 0 {
		cacheableResponse.BaseResponse.Warnings = append(cacheableResponse.BaseResponse.Warnings, "empty result")
		return nil, fmt.Errorf("%w: %s", err, toUnreadableBodyMessage(uri, []byte(cacheableResponse.String())))
	}

	if findResponse.Status != "success" && findResponse.Status != "pending" {
		cacheableResponse.BaseResponse.Warnings = append(cacheableResponse.BaseResponse.Warnings, "invalid zoneConfigsFindResponse status")
		return findResponse, errors.New(toUnreadableBodyMessage(uri, []byte(cacheableResponse.String())))
	}

	return findResponse, nil
}

// https://www.hosting.de/api/?json#updating-zones
func (d *hostingdeDNSProviderSolver) updateZone(updateRequest ZoneUpdateRequest) (*ZoneUpdateResponse, error) {
	uri := defaultBaseURL + "/zoneUpdate"

	// but we'll need the ID later to delete the record
	updateResponse := &ZoneUpdateResponse{}

	_, err := d.post(uri, updateRequest, updateResponse)
	if err != nil {
		return nil, err
	}

	cacheableResponse := ZoneUpdateResponse{
		BaseResponse: BaseResponse{
			Errors:   updateResponse.Errors,
			Warnings: append(updateResponse.Warnings, "invalid updateResponse status"),
			Status:   updateResponse.Status,
		},
		Response: updateResponse.Response,
	}

	if updateResponse.Status != "success" && updateResponse.Status != "pending" {
		return nil, errors.New(toUnreadableBodyMessage(uri, []byte(cacheableResponse.String())))
	}

	return updateResponse, nil
}

func (d *hostingdeDNSProviderSolver) getZone(findRequest ZoneConfigsFindRequest) (*ZoneConfig, error) {
	ctx, cancel := context.WithCancel(context.Background())

	var zoneConfig *ZoneConfig

	operation := func() error {
		findResponse, err := d.listZoneConfigs(findRequest)
		if err != nil {
			cancel()
			return err
		}

		if findResponse.Response.Data[0].Status != "active" {
			return fmt.Errorf("unexpected status: %q", findResponse.Response.Data[0].Status)
		}

		zoneConfig = &findResponse.Response.Data[0]

		return nil
	}

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 3 * time.Second
	bo.MaxInterval = 10 * bo.InitialInterval
	bo.MaxElapsedTime = 100 * bo.InitialInterval

	// retry in case the zone was edited recently and is not yet active
	err := backoff.Retry(operation, backoff.WithContext(bo, ctx))
	if err != nil {
		return nil, err
	}

	return zoneConfig, nil
}

func (d *hostingdeDNSProviderSolver) post(uri string, request interface{}, response interface{}) ([]byte, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, uri, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error querying API: %w", err)
	}

	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.New(toUnreadableBodyMessage(uri, content))
	}

	err = json.Unmarshal(content, response)
	if err != nil {
		// errors here are still not guaranteed to be cacheable
		// p.e. if Unmarshal failed due to changes on the API response format
		return nil, fmt.Errorf("%w: %s", err, toUnreadableBodyMessage(uri, content))
	}

	return content, nil
}

func toUnreadableBodyMessage(uri string, rawBody []byte) string {
	baseResp := &BaseResponse{}
	if err := json.Unmarshal(rawBody, &baseResp); err == nil {
		// try to extract errors from response
		if len(baseResp.Errors) > 0 {
			if clean, err := json.Marshal(baseResp); err == nil {
				return fmt.Sprintf("the request %s sent a response with errors: %q", uri, clean)
			}
		}
	}

	return fmt.Sprintf("the request %s sent a response with a body which is an invalid format: %q", uri, rawBody)
}
