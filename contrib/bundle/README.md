# CRI-O static build bundle

To install the bundle, run the following from a development sandbox:

```shell
sudo ./install
```

After that, it should be possible to start CRI-O by executing:

```shell
sudo systemctl daemon-reload
sudo systemctl enable --now crio
```
