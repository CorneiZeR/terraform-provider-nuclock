default: test

test:
	go test ./...

# Against a real nuclock. NUCLOCK_URL and NUCLOCK_API_KEY must be set: a
# service key with projects.write, people.write, seats.write, integrations.write.
testacc:
	cd examples/acceptance && terraform init -upgrade && terraform apply && terraform plan -detailed-exitcode

docs:
	go generate ./...

.PHONY: default test testacc docs
