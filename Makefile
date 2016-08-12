.PHONY: all clean ocid ocic

all: ocid ocic

ocid:
	go build -o ocid ./cmd/server/main.go

ocic:
	go build -o ocic ./cmd/client/main.go

clean:
	rm -f ocic ocid
