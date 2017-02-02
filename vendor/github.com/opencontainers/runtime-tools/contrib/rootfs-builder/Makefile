rootfs.tar.gz: rootfs/bin/echo
	tar -czf $@ -C rootfs .

rootfs/bin/busybox: downloads/stage3-amd64-current.tar.bz2 rootfs-files
	gpg --verify $<.DIGESTS.asc
	(cd downloads && \
		grep -A1 '^# SHA512 HASH' stage3-amd64-current.tar.bz2.DIGESTS.asc | \
		grep -v '^--' | \
		sha512sum -c)
	sudo rm -rf rootfs
	sudo mkdir rootfs
	sudo tar -xvf downloads/stage3-amd64-current.tar.bz2 -C rootfs \
		--no-recursion --wildcards $$(< rootfs-files)
	sudo touch $@

rootfs/bin/echo: rootfs/bin/busybox
	sudo sh -c 'for COMMAND in $$($< --list); do \
		ln -rs $< "rootfs/bin/$${COMMAND}"; \
	done'

downloads/stage3-amd64-current.tar.bz2: get-stage3.sh
	./$<
	touch downloads/stage3-amd64-*.tar.bz2

clean:
	rm -f downloads/*
	sudo rm -rf rootfs
