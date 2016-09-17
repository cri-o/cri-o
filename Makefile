.PHONY: all clean conmon ocid ocic update-deps

all: conmon ocid ocic

conmon:
	make -C $@

ocid:
	go build -o ocid ./cmd/server/main.go

ocic:
	go build -o ocic ./cmd/client/main.go

clean:
	rm -f ocic ocid

update-deps:
	@which glide > /dev/null 2>/dev/null || (echo "ERROR: glide not found." && false)
	glide update --strip-vcs --strip-vendor --update-vendored --delete
	glide-vc --only-code --no-tests
	# see http://sed.sourceforge.net/sed1line.txt
	find vendor -type f -exec sed -i -e :a -e '/^\n*$$/{$$d;N;ba' -e '}' "{}" \;
