.PHONY: all clean conmon ocid ocic

all: conmon ocid ocic

conmon:
	make -C $@

ocid:
	go build -o ocid ./cmd/server/main.go

ocic:
	go build -o ocic ./cmd/client/main.go

clean:
	rm -f ocic ocid
