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

DEFAULT_BINARY ?= crio-x86_64-static-musl

all: install

.PHONY: install
install:
	install $(SELINUX) -d -m 755 $(BASHINSTALLDIR)
	install $(SELINUX) -d -m 755 $(FISHINSTALLDIR)
	install $(SELINUX) -d -m 755 $(ZSHINSTALLDIR)
	install $(SELINUX) -d -m 755 $(CONTAINERS_DIR)
	install $(SELINUX) -d -m 755 $(CNIDIR)
	install $(SELINUX) -D -m 755 -t $(BINDIR) bin/crio-x86_64-static-glibc
	install $(SELINUX) -D -m 755 -t $(BINDIR) bin/crio-x86_64-static-musl
	ln -sf $(BINDIR)/$(DEFAULT_BINARY) $(BINDIR)/crio
	install $(SELINUX) -D -m 755 -t $(BINDIR) bin/pause-x86_64-static-glibc
	install $(SELINUX) -D -m 755 -t $(BINDIR) bin/pause-x86_64-static-musl
	install $(SELINUX) -D -m 644 -t $(ETCDIR) etc/crictl.yaml
	install $(SELINUX) -D -m 644 -t $(OCIDIR) etc/crio-umount.conf
	install $(SELINUX) -D -m 644 -t $(ETCDIR)/crio etc/crio.conf
	install $(SELINUX) -D -m 644 -t $(MANDIR)/man5 man/crio.conf.5
	install $(SELINUX) -D -m 644 -t $(MANDIR)/man8 man/crio.8
	install $(SELINUX) -D -m 644 -t $(BASHINSTALLDIR) completions/bash/crio
	install $(SELINUX) -D -m 644 -t $(FISHINSTALLDIR) completions/fish/crio.fish
	install $(SELINUX) -D -m 644 -t $(ZSHINSTALLDIR) completions/zsh/_crio
	install $(SELINUX) -D -m 644 -t $(CNIDIR) contrib/10-crio-bridge.conf
	install $(SELINUX) -D -m 644 -t $(CONTAINERS_DIR) contrib/policy.json
	install $(SELINUX) -D -m 644 -t $(SYSTEMDDIR) contrib/crio-shutdown.service
	install $(SELINUX) -D -m 644 -t $(SYSTEMDDIR) contrib/crio-wipe.service
	install $(SELINUX) -D -m 644 -t $(SYSTEMDDIR) contrib/crio.service

.PHONY: uninstall
uninstall:
	rm $(BINDIR)/crio
	rm $(BINDIR)/crio-x86_64-static-glibc
	rm $(BINDIR)/crio-x86_64-static-musl
	rm $(BINDIR)/pause-x86_64-static-glibc
	rm $(BINDIR)/pause-x86_64-static-musl
	rm $(ETCDIR)/crictl.yaml
	rm $(OCIDIR)/crio-umount.conf
	rm $(ETCDIR)/crio/crio.conf
	rm $(MANDIR)/man5/crio.conf.5
	rm $(MANDIR)/man8/crio.8
	rm $(BASHINSTALLDIR)/crio
	rm $(FISHINSTALLDIR)/crio.fish
	rm $(ZSHINSTALLDIR)/_crio
	rm $(CNIDIR)/10-crio-bridge.conf
	rm $(CONTAINERS_DIR)/policy.json
	rm $(SYSTEMDDIR)/crio-shutdown.service
	rm $(SYSTEMDDIR)/crio-wipe.service
	rm $(SYSTEMDDIR)/crio.service
