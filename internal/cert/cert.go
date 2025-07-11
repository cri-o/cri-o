package cert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"k8s.io/client-go/util/cert"

	"github.com/cri-o/cri-o/internal/log"
)

type Config struct {
	mu     sync.RWMutex
	config *tls.Config

	TLSCert string
	TLSKey  string
	TLSCA   string
}

func NewCertConfig(ctx context.Context, doneChan chan struct{}, certPath, keyPath, caPath string) (*Config, error) {
	cc := &Config{
		TLSCert: certPath,
		TLSKey:  keyPath,
		TLSCA:   caPath,
	}

	if err := cc.reload(ctx); err != nil {
		return nil, fmt.Errorf("reload certificates: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	go func() {
		defer watcher.Close()

		done := make(chan struct{})

		go func() {
			for {
				select {
				case event := <-watcher.Events:
					log.Debugf(
						ctx,
						"Got cert watcher event for %s (%s), reloading certificates",
						event.Name, event.Op.String(),
					)

					if err := cc.reload(ctx); err != nil {
						log.Warnf(ctx, "Keeping previous certificates: %v", err)
					}
				case err := <-watcher.Errors:
					log.Errorf(ctx, "Cert watcher error: %v", err)
					close(done)

					return
				case <-doneChan:
					log.Debugf(ctx, "Closing cert watcher")
					close(done)

					return
				}
			}
		}()

		for _, f := range []string{certPath, keyPath, caPath} {
			log.Debugf(ctx, "Watching file %s for changes", f)

			if err := watcher.Add(f); err != nil {
				log.Fatalf(ctx, "Unable to watch %s: %v", f, err)
			}
		}

		<-done
	}()

	return cc, nil
}

// GetConfigForClient gets the tlsConfig for the streaming server.
// This allows the certs to be swapped, without shutting down crio.
func (cc *Config) GetConfigForClient(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	return cc.config, nil
}

func (cc *Config) reload(ctx context.Context) error {
	config := new(tls.Config)
	certificate, err := tls.LoadX509KeyPair(cc.TLSCert, cc.TLSKey)
	// Validate the certificates
	if err != nil {
		return fmt.Errorf("load x509 key pair: %w", err)
	}

	if len(certificate.Certificate) == 0 {
		return errors.New("certificates chain is empty")
	}

	x509Cert, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return fmt.Errorf("parse x509 certificate: %w", err)
	}

	log.Infof(
		ctx,
		"Metrics certificate is valid between %v and %v",
		x509Cert.NotBefore, x509Cert.NotAfter,
	)

	now := time.Now()
	if now.After(x509Cert.NotAfter) {
		return errors.New("certificate is not valid any more")
	}

	if now.Before(x509Cert.NotBefore) {
		return errors.New("certificate is not yet valid")
	}

	config.Certificates = []tls.Certificate{certificate}

	// Set up mTLS configurations if TLSCA is set
	if cc.TLSCA != "" {
		caBytes, err := os.ReadFile(cc.TLSCA)
		if err != nil {
			return fmt.Errorf("read TLS CA file: %w", err)
		}

		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(caBytes)
		config.ClientCAs = certPool
		config.ClientAuth = tls.RequireAndVerifyClientCert
	}

	cc.mu.Lock()
	cc.config = config
	cc.mu.Unlock()

	return nil
}

func GenerateSelfSignedCertKey(ctx context.Context, certPath, keyPath string) error {
	_, errCertPath := os.Stat(certPath)

	_, errKeyPath := os.Stat(keyPath)
	if errCertPath != nil && os.IsNotExist(errCertPath) && errKeyPath != nil && os.IsNotExist(errKeyPath) {
		log.Infof(ctx, "Metrics key and cert path does not exist, generating self-signed")

		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("retrieve hostname: %w", err)
		}

		certBytes, keyBytes, err := cert.GenerateSelfSignedCertKey(hostname, nil, nil)
		if err != nil {
			return fmt.Errorf("generate self-signed cert/key: %w", err)
		}

		for path, bytes := range map[string][]byte{
			certPath: certBytes,
			keyPath:  keyBytes,
		} {
			if err := os.MkdirAll(filepath.Dir(path), os.FileMode(0o700)); err != nil {
				return fmt.Errorf("create path: %w", err)
			}

			if err := os.WriteFile(path, bytes, os.FileMode(0o600)); err != nil {
				return fmt.Errorf("write file: %w", err)
			}
		}
	}

	return nil
}
