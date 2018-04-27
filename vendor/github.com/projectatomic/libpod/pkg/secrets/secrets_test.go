package secrets

import (
	"testing"
)

const (
	defaultError   = "unable to get host and container dir"
	secretDataPath = "fixtures/secret"
	emptyPath      = "fixtures/secret/empty"
)

func TestGetMountsMap(t *testing.T) {
	testCases := []struct {
		Path, HostDir, CtrDir string
		Error                 string
	}{
		{"", "", "", defaultError},
		{"/tmp:/home/crio", "/tmp", "/home/crio", ""},
		{"crio/logs:crio/logs", "crio/logs", "crio/logs", ""},
		{"/tmp", "", "", defaultError},
	}
	for _, c := range testCases {
		hostDir, ctrDir, err := getMountsMap(c.Path)
		if hostDir != c.HostDir || ctrDir != c.CtrDir || (err != nil && err.Error() != c.Error) {
			t.Errorf("expect: (%v, %v, %v) \n but got: (%v, %v, %v) \n",
				c.HostDir, c.CtrDir, c.Error, hostDir, ctrDir, err)
		}
	}
}

func TestGetHostSecretData(t *testing.T) {
	testCases := []struct {
		Path string
		Want []secretData
	}{
		{
			"emptyPath",
			[]secretData{},
		},
		{
			secretDataPath,
			[]secretData{
				{"testDataA", []byte("secretDataA")},
				{"testDataB", []byte("secretDataB")},
			},
		},
	}
	for _, c := range testCases {
		if secretData, err := getHostSecretData(c.Path); err != nil {
			t.Error(err)
		} else {
			for index, data := range secretData {
				if data.name != c.Want[index].name || string(data.data) != string(c.Want[index].data) {
					t.Errorf("expect: (%v, %v) \n but got: (%v, %v) \n",
						c.Want[index].name, string(c.Want[index].data), data.name, string(data.data))
				}
			}
		}
	}
}
