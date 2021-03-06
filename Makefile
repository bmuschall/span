TARGETS = span-import span-export span-gh-dump

# http://docs.travis-ci.com/user/languages/go/#Default-Test-Script
test: assets deps
	go test -v ./...

deps:
	go get ./...

imports:
	goimports -w .

assets: assetutil/bindata.go

assetutil/bindata.go:
	go get -f -u github.com/jteeuwen/go-bindata/...
	go-bindata -o assetutil/bindata.go -pkg assetutil assets/...

cover:
	go test -cover ./...

all: $(TARGETS)

span-import: assets imports deps
	go build -o span-import cmd/span-import/main.go

span-export: assets imports deps
	go build -o span-export cmd/span-export/main.go

span-gh-dump: assets imports deps
	go build -o span-gh-dump cmd/span-gh-dump/main.go

clean:
	rm -f $(TARGETS)
	rm -f span_*deb
	rm -f span-*rpm
	rm -rf ./packaging/deb/span/usr
	rm -f assetutil/bindata.go

deb: $(TARGETS)
	mkdir -p packaging/deb/span/usr/sbin
	cp $(TARGETS) packaging/deb/span/usr/sbin
	cd packaging/deb && fakeroot dpkg-deb --build span .
	mv packaging/deb/span_*.deb .

rpm: $(TARGETS)
	mkdir -p $(HOME)/rpmbuild/{BUILD,SOURCES,SPECS,RPMS}
	cp ./packaging/rpm/span.spec $(HOME)/rpmbuild/SPECS
	cp $(TARGETS) $(HOME)/rpmbuild/BUILD
	./packaging/rpm/buildrpm.sh span
	cp $(HOME)/rpmbuild/RPMS/x86_64/span*.rpm .

cloc:
	cloc --max-file-size 1 --exclude-dir assets --exclude-dir assetutil --exclude-dir tmp --exclude-dir fixtures .
