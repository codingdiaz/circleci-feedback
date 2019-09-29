build: clean
	env GOOS=linux go build -ldflags="-s -w" -o bin/entry cmd/entry/*.go
	env GOOS=linux go build -ldflags="-s -w" -o bin/find_pipeline_id cmd/find_pipeline_id/*.go
	env GOOS=linux go build -ldflags="-s -w" -o bin/wait_for_jobs cmd/wait_for_jobs/*.go

clean:
	rm -rf ./bin

deploy: clean build
	sls deploy --verbose

fmt:
	go fmt ./pkg... ./internal... ./cmd...

deploy-docs:
	cd docs && mkdocs gh-deploy
