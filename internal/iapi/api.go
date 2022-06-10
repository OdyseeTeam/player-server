package iapi

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/bluele/gcache"
	"github.com/lbryio/lbry.go/v2/extras/errors"
)

type BlockedContent struct {
	ClaimID  string `json:"claim_id"`
	Outpoint string `json:"outpoint"`
}

var blockedCache = gcache.New(10).Expiration(2 * time.Minute).Build()

func GetBlockedContent() (map[string]bool, error) {
	cachedVal, err := blockedCache.Get("blocked")
	if err == nil && cachedVal != nil {
		return cachedVal.(map[string]bool), nil
	}

	url := "https://api.odysee.com/file/list_blocked?with_claim_id=true"
	method := "GET"
	type APIResponse struct {
		Data []BlockedContent
	}

	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)

	if err != nil {
		return nil, errors.Err(err)
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.Err(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.Err("unexpected status code %d", res.StatusCode)
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Err(err)
	}
	var response APIResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, errors.Err(err)
	}
	blockedMap := make(map[string]bool, len(response.Data))
	for _, bc := range response.Data {
		blockedMap[bc.ClaimID] = true
	}
	err = blockedCache.Set("blocked", blockedMap)
	if err != nil {
		return blockedMap, errors.Err(err)
	}
	return blockedMap, nil
}
