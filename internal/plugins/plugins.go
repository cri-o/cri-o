package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/cri-o/cri-o/internal/log"
	v1 "github.com/cri-o/cri-o/pkg/plugin/v1"
)

const pluginDir = "/etc/crio/plugins"

// Plugins holds the arbitrary data for all loaded plugins.
type Plugins struct {
	servicePlugin *v1.ServicePlugin
	plugins       *sync.Map
}

var instance *Plugins

// Instance returns the singleton instance of the Plugins.
func Instance() *Plugins {
	if instance == nil {
		instance = &Plugins{plugins: &sync.Map{}}
	}
	return instance
}

// Load loads all available plugins.
func (p *Plugins) Load(ctx context.Context) error {
	servicePlugin, err := v1.NewServicePlugin(ctx)
	if err != nil {
		return fmt.Errorf("create new WASM service plugin: %w", err)
	}

	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return fmt.Errorf("ensure WASM plugin directory %s: %w", pluginDir, err)
	}

	log.Infof(ctx, "Loading WASM plugins")
	plugins := &sync.Map{}
	if err := filepath.Walk(pluginDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		service, err := servicePlugin.Load(ctx, path)
		if err != nil {
			return fmt.Errorf("load WASM service plugin %s: %w", path, err)
		}

		name := filepath.Base(path)
		log.Infof(ctx, "Loaded WASM plugin %q from path: %s", name, path)
		plugins.Store(name, service)

		return nil
	}); err != nil {
		return fmt.Errorf("walk WASM plugin directory %s: %w", pluginDir, err)
	}

	p.servicePlugin = servicePlugin
	p.plugins = plugins

	return nil
}

func (p *Plugins) Run(
	ctx context.Context,
	method string,
	rpcType v1.RPCRequest_Type,
	rpcData any,
	rpcError error,
) (any, error) {
	p.plugins.Range(func(key, value any) bool {
		plugin, pluginOk := value.(v1.Service)
		name, nameOk := key.(string)
		if !(pluginOk && nameOk) {
			log.Errorf(ctx, "Type assertion error for plugin %v (key = %v), please report a bug", value, key)
			return true
		}

		log.Debugf(ctx, "Running RPC plugin %q (%s) for method: %s", name, v1.RPCRequest_Type_name[int32(rpcType)], method)

		rpcJSON, err := json.Marshal(rpcData)
		if err != nil {
			log.Errorf(ctx, "Unable to marshal RPC data to JSON bytes: %v", err)
			return false
		}

		response, err := plugin.RPC(ctx, &v1.RPCRequest{
			Type:   rpcType,
			Method: method,
			Json:   rpcJSON,
			Error:  errorToString(rpcError),
		})
		if err != nil {
			log.Errorf(ctx, "Unable to execute RPC plugin %q (removing it): %v", name, err)
			p.plugins.Delete(key)
			return true
		}

		if response.GetJson() != nil {
			if err := json.Unmarshal(response.GetJson(), rpcData); err != nil {
				log.Errorf(ctx, "Unable to unmarshal JSON from new RPC data: %v", err)
			} else {
				log.Debugf(ctx, "Plugin %q modified RPC data: %+v", name, rpcData)
			}
		}

		if response.GetError() != "" {
			rpcError = errors.New(response.GetError())
			log.Debugf(ctx, "Plugin %q modified RPC error: %v", name, rpcError)
		}

		return true
	})

	return rpcData, rpcError
}

func errorToString(rpcError error) string {
	if rpcError == nil {
		return ""
	}
	return rpcError.Error()
}
