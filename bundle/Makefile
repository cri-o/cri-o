PREFIX ?= $(DESTDIR)/usr/local
ETCDIR ?= $(DESTDIR)/etc
BINDIR ?= $(PREFIX)/bin
MANDIR ?= $(PREFIX)/share/man
OCIDIR ?= $(PREFIX)/share/oci-umount/oci-umount.d
SELINUX ?= $(shell selinuxenabled 2>/dev/null && echo -Z)

all: install

.PHONY: install
install:
	install $(SELINUX) -D -m 755 -t $(BINDIR) bin/crio-x86_64-static-glibc
	install $(SELINUX) -D -m 755 -t $(BINDIR) bin/crio-x86_64-static-musl
	install $(SELINUX) -D -m 755 -t $(BINDIR) bin/pause-x86_64-static-glibc
	install $(SELINUX) -D -m 755 -t $(BINDIR) bin/pause-x86_64-static-musl
	install $(SELINUX) -D -m 644 -t $(ETCDIR) etc/crictl.yaml
	install $(SELINUX) -D -m 644 -t $(OCIDIR) etc/crio-umount.conf
	install $(SELINUX) -D -m 644 -t $(ETCDIR)/crio etc/crio.conf
	install $(SELINUX) -D -m 644 -t $(MANDIR)/man5 man/crio.conf.5
	install $(SELINUX) -D -m 644 -t $(MANDIR)/man8 man/crio.8

.PHONY: uninstall
uninstall:
	rm $(BINDIR)/crio-x86_64-static-glibc
	rm $(BINDIR)/crio-x86_64-static-musl
	rm $(BINDIR)/pause-x86_64-static-glibc
	rm $(BINDIR)/pause-x86_64-static-musl
	rm $(ETCDIR)/crictl.yaml
	rm $(OCIDIR)/crio-umount.conf
	rm $(ETCDIR)/crio/crio.conf
	rm $(MANDIR)/man5/crio.conf.5
	rm $(MANDIR)/man8/crio.8
