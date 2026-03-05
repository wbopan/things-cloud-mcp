package thingscloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// AppInstanceRequest describes the payload for registering a device for push notifications
type AppInstanceRequest struct {
	AppInstanceID string `json:"-"`
	HistoryKey    string `json:"history-key"`
	APNSToken     string `json:"apns-token"`
	AppID         string `json:"app-id"`
	Dev           bool   `json:"dev"`
}

// RegisterAppInstance registers a device for push notifications via APNS
func (c *Client) RegisterAppInstance(req AppInstanceRequest) error {
	bs, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequest("PUT",
		fmt.Sprintf("/version/1/app-instance/%s", req.AppInstanceID), bytes.NewReader(bs))
	if err != nil {
		return err
	}
	resp, err := c.do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http response code: %s", resp.Status)
	}
	return nil
}
