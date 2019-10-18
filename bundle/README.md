# CRI-O static build bundle

The following runtime dependencies are needed to let CRI-O work in conjunction
with this bundle:

- [runc][0]
- [conmon][1]
- [CNI plugins][2]

[0]: https://github.com/opencontainers/runc
[1]: https://github.com/containers/conmon
[2]: https://github.com/containernetworking/plugins

To install the bundle, run:

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
