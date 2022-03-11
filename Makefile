.PHONY: build clean deploy

build:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/newmovies newmovies/main.go

clean:
	rm -rf ./bin

deploy: clean build
	sls deploy --verbose
