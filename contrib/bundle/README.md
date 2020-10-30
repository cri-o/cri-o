# CRI-O static build bundle

To install the bundle, run the following from a development sandbox:

```
$ sudo make install
```

After that, it should be possible to start CRI-O by executing:

```
$ sudo systemctl daemon-reload
$ sudo systemctl enable --now crio
```

To uninstall the bundle, run:

```
$ sudo make uninstall
```
