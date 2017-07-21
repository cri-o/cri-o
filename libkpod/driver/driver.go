package driver

import cstorage "github.com/containers/storage"

type DriverData struct {
	Name string
	Data map[string]string
}

func GetDriverName(store cstorage.Store) (string, error) {
	driver, err := store.GraphDriver()
	if err != nil {
		return "", err
	}
	return driver.String(), nil
}

func GetDriverMetadata(store cstorage.Store, layerID string) (map[string]string, error) {
	driver, err := store.GraphDriver()
	if err != nil {
		return nil, err
	}
	return driver.Metadata(layerID)
}
