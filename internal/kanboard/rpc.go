package kanboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
	"io"
	"net/http"
	"time"
)

func WebClientRpcCall[REQ any, RESP any](kbCfg config.KanboardConfig, req REQ, resp *RESP) error {
	// Connect to server
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	// https://docs.kanboard.org/en/1.2.22/api/authentication.html
	client := &http.Client{
		Timeout: time.Second * 10,
	}
	httpReq, err := http.NewRequest("POST", kbCfg.ApiUrl, bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.SetBasicAuth(kbCfg.Username, kbCfg.Password)
	httpReq.Header.Set("Content-Type", "application/json")
	response, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	//bodyString := string(bodyBytes)
	//println(bodyString)

	var rpcErr KBResponseError
	err = json.Unmarshal(bodyBytes, &rpcErr)
	if err == nil {
		if rpcErr.JsonRpc != "2.0" {
			return fmt.Errorf(
				"json rpc protocol error, got jsonrpc=%v instead of 2.0", rpcErr.JsonRpc)
		}
		if rpcErr.Error.Message != "" {
			return fmt.Errorf(
				"error httpStatus=%v errorCode=%v errorMessage=%v data=%v",
				response.StatusCode, rpcErr.Error.Code, rpcErr.Error.Message, rpcErr.Error.Data)
		}
	}
	err = json.Unmarshal(bodyBytes, &resp)
	if err != nil {
		return err
	}
	return nil
}
