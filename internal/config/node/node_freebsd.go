package node

// ValidateConfig initializes and validates all of the singleton variables
// that store the node's configuration. Nothing here for FreeBSD yet.
// We check the error at server configuration validation, and if we error, shutdown
// cri-o early, instead of when we're already trying to run containers.
func ValidateConfig() error {
	return nil
}
