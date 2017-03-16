In terminal 1:
```
sudo ./ocid
```

In terminal 2:
```
sudo ./ocic runtimeversion

sudo rm -rf /var/lib/containers/storage/sandboxes/podsandbox1
sudo ./ocic pod run --config testdata/sandbox_config.json

sudo rm -rf /var/lib/containers/storage/containers/container1
sudo ./ocic container create --pod podsandbox1 --config testdata/container_config.json
```
