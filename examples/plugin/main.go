//go:build tinygo.wasm

package main

import (
	"context"
	"encoding/json"
	"fmt"

	v1 "github.com/cri-o/cri-o/pkg/plugin/v1"
)

func main() {
	v1.RegisterService(plugin{})
}

type plugin struct{}

const rpcMethod = "/runtime.v1.RuntimeService/Version"

func (p plugin) RPC(_ context.Context, request *v1.RPCRequest) (*v1.RPCResponse, error) {
	if request.GetMethod() != rpcMethod || request.GetType() != v1.RPCRequest_TYPE_POST {
		return nil, nil
	}

	var rm map[string]any
	if err := json.Unmarshal(request.GetJson(), &rm); err != nil {
		return nil, fmt.Errorf("unable to unmarshal JSON: %w", err)
	}

	rm["runtime_name"] = "my-new-runtime"
	rm["runtime_api_version"] = "v2"

	newJSON, err := json.Marshal(rm)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal new JSON: %w", err)
	}

	return &v1.RPCResponse{
		Json: newJSON,
	}, nil
}
