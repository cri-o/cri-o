PREFIX ?= $(DESTDIR)/usr/local
ETCDIR ?= $(DESTDIR)/etc
CONTAINERS_DIR ?= $(ETCDIR)/containers
CNIDIR ?= $(ETCDIR)/cni/net.d
BINDIR ?= $(PREFIX)/bin
MANDIR ?= $(PREFIX)/share/man
OCIDIR ?= $(PREFIX)/share/oci-umount/oci-umount.d
SELINUX ?= $(shell selinuxenabled 2>/dev/null && echo -Z)
BASHINSTALLDIR ?= $(PREFIX)/share/bash-completion/completions
FISHINSTALLDIR ?= $(PREFIX)/share/fish/completions
ZSHINSTALLDIR ?= $(PREFIX)/share/zsh/site-functions
SYSTEMDDIR ?= $(PREFIX)/lib/systemd/system
OPT_CNI_BIN_DIR ?= /opt/cni/bin

all: install

.PHONY: install
install: \
	install-cni \
	install-conmon \
	install-crio \
	install-crictl \
	install-pinns \
	install-runc

.PHONY: install-cni
install-cni:
	install $(SELINUX) -d -m 755 $(CNIDIR)
	install $(SELINUX) -D -m 755 -t $(OPT_CNI_BIN_DIR) cni-plugins/*
	install $(SELINUX) -D -m 644 -t $(CNIDIR) contrib/10-crio-bridge.conf

.PHONY: install-conmon
install-conmon:
	install $(SELINUX) -D -m 755 -t $(BINDIR) bin/conmon

.PHONY: install-crictl
install-crictl:
	install $(SELINUX) -D -m 755 -t $(BINDIR) bin/crictl

.PHONY: install-crio
install-crio:
	install $(SELINUX) -d -m 755 $(BASHINSTALLDIR)
	install $(SELINUX) -d -m 755 $(FISHINSTALLDIR)
	install $(SELINUX) -d -m 755 $(ZSHINSTALLDIR)
	install $(SELINUX) -d -m 755 $(CONTAINERS_DIR)
	install $(SELINUX) -D -m 755 -t $(BINDIR) bin/crio-status-static
	install $(SELINUX) -D -m 755 -t $(BINDIR) bin/crio-static
	ln -sf $(BINDIR)/crio-static $(BINDIR)/crio
	install $(SELINUX) -D -m 644 -t $(ETCDIR) etc/crictl.yaml
	install $(SELINUX) -D -m 644 -t $(OCIDIR) etc/crio-umount.conf
	install $(SELINUX) -D -m 644 -t $(ETCDIR)/crio etc/crio.conf
	install $(SELINUX) -D -m 644 -t $(MANDIR)/man5 man/crio.conf.5
	install $(SELINUX) -D -m 644 -t $(MANDIR)/man5 man/crio.conf.d.5
	install $(SELINUX) -D -m 644 -t $(MANDIR)/man8 man/crio.8
	install $(SELINUX) -D -m 644 -t $(BASHINSTALLDIR) completions/bash/crio
	install $(SELINUX) -D -m 644 -t $(FISHINSTALLDIR) completions/fish/crio.fish
	install $(SELINUX) -D -m 644 -t $(ZSHINSTALLDIR) completions/zsh/_crio
	install $(SELINUX) -D -m 644 -t $(CONTAINERS_DIR) contrib/policy.json
	install $(SELINUX) -D -m 644 -t $(SYSTEMDDIR) contrib/crio.service

.PHONY: install-pinns
install-pinns:
	install $(SELINUX) -D -m 755 -t $(BINDIR) bin/pinns

.PHONY: install-runc
install-runc:
	install $(SELINUX) -D -m 755 -t $(BINDIR) bin/runc

.PHONY: uninstall
uninstall: \
	uninstall-cni \
	uninstall-conmon \
	uninstall-crio \
	uninstall-crictl \
	uninstall-pinns \
	uninstall-runc

.PHONY: uninstall-cni
uninstall-cni:
	rm $(OPT_CNI_BIN_DIR)/*
	rm $(CNIDIR)/10-crio-bridge.conf

.PHONY: uninstall-conmon
uninstall-conmon:
	rm $(BINDIR)/conmon

.PHONY: uninstall-crictl
uninstall-crictl:
	rm $(BINDIR)/crictl

.PHONY: uninstall-crio
uninstall-crio:
	rm $(BINDIR)/crio
	rm $(BINDIR)/crio-status-static
	rm $(BINDIR)/crio-static
	rm $(ETCDIR)/crictl.yaml
	rm $(OCIDIR)/crio-umount.conf
	rm $(ETCDIR)/crio/crio.conf
	rm $(MANDIR)/man5/crio.conf.5
	rm $(MANDIR)/man5/crio.conf.d.5
	rm $(MANDIR)/man8/crio.8
	rm $(BASHINSTALLDIR)/crio
	rm $(FISHINSTALLDIR)/crio.fish
	rm $(ZSHINSTALLDIR)/_crio
	rm $(CONTAINERS_DIR)/policy.json
	rm $(SYSTEMDDIR)/crio.service

.PHONY: uninstall-pinns
uninstall-pinns:
	rm $(BINDIR)/pinns

.PHONY: uninstall-runc
uninstall-runc:
	rm $(BINDIR)/runc
