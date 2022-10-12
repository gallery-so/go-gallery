cloud-tasks : export APP_ENGINE_EMULATOR_HOST := http://localhost:8080


contracts: solc abi-gen

solc:
	solc --abi ./contracts/sol/IERC721.sol > ./contracts/abi/IERC721.abi
	solc --abi ./contracts/sol/IERC20.sol > ./contracts/abi/IERC20.abi
	solc --abi ./contracts/sol/IERC721Metadata.sol > ./contracts/abi/IERC721Metadata.abi
	solc --abi ./contracts/sol/IERC1155.sol > ./contracts/abi/IERC1155.abi
	solc --abi ./contracts/sol/IENS.sol > ./contracts/abi/IENS.abi
	solc --abi ./contracts/sol/IERC1155Metadata_URI.sol > ./contracts/abi/IERC1155Metadata_URI.abi
	solc --abi ./contracts/sol/ISignatureValidator.sol > ./contracts/abi/ISignatureValidator.abi
	solc --abi ./contracts/sol/CryptopunksData.sol > ./contracts/abi/CryptopunksData.abi
	solc --abi ./contracts/sol/Cryptopunks.sol > ./contracts/abi/Cryptopunks.abi
	solc --abi ./contracts/sol/Zora.sol > ./contracts/abi/Zora.abi
	tail -n +4 "./contracts/abi/IERC721.abi" > "./contracts/abi/IERC721.abi.tmp" && mv "./contracts/abi/IERC721.abi.tmp" "./contracts/abi/IERC721.abi"
	tail -n +4 "./contracts/abi/IERC20.abi" > "./contracts/abi/IERC20.abi.tmp" && mv "./contracts/abi/IERC20.abi.tmp" "./contracts/abi/IERC20.abi"
	tail -n +4 "./contracts/abi/IERC721Metadata.abi" > "./contracts/abi/IERC721Metadata.abi.tmp" && mv "./contracts/abi/IERC721Metadata.abi.tmp" "./contracts/abi/IERC721Metadata.abi"
	tail -n +4 "./contracts/abi/IERC1155.abi" > "./contracts/abi/IERC1155.abi.tmp" && mv "./contracts/abi/IERC1155.abi.tmp" "./contracts/abi/IERC1155.abi"
	tail -n +4 "./contracts/abi/IENS.abi" > "./contracts/abi/IENS.abi.tmp" && mv "./contracts/abi/IENS.abi.tmp" "./contracts/abi/IENS.abi"
	tail -n +4 "./contracts/abi/IERC1155Metadata_URI.abi" > "./contracts/abi/IERC1155Metadata_URI.abi.tmp" && mv "./contracts/abi/IERC1155Metadata_URI.abi.tmp" "./contracts/abi/IERC1155Metadata_URI.abi"
	tail -n +4 "./contracts/abi/ISignatureValidator.abi" > "./contracts/abi/ISignatureValidator.abi.tmp" && mv "./contracts/abi/ISignatureValidator.abi.tmp" "./contracts/abi/ISignatureValidator.abi"
	tail -n +4 "./contracts/abi/CryptopunksData.abi" > "./contracts/abi/CryptopunksData.abi.tmp" && mv "./contracts/abi/CryptopunksData.abi.tmp" "./contracts/abi/CryptopunksData.abi"
	tail -n +4 "./contracts/abi/Cryptopunks.abi" > "./contracts/abi/Cryptopunks.abi.tmp" && mv "./contracts/abi/Cryptopunks.abi.tmp" "./contracts/abi/Cryptopunks.abi"
	tail -n +4 "./contracts/abi/Zora.abi" > "./contracts/abi/Zora.abi.tmp" && mv "./contracts/abi/Zora.abi.tmp" "./contracts/abi/Zora.abi"
abi-gen:
	abigen --abi=./contracts/abi/IERC721.abi --pkg=contracts --type=IERC721 > ./contracts/IERC721.go
	abigen --abi=./contracts/abi/IERC20.abi --pkg=contracts --type=IERC20 > ./contracts/IERC20.go
	abigen --abi=./contracts/abi/IERC721Metadata.abi --pkg=contracts --type=IERC721Metadata > ./contracts/IERC721Metadata.go
	abigen --abi=./contracts/abi/IERC1155.abi --pkg=contracts --type=IERC1155 > ./contracts/IERC1155.go
	abigen --abi=./contracts/abi/IENS.abi --pkg=contracts --type=IENS > ./contracts/IENS.go
	abigen --abi=./contracts/abi/IERC1155Metadata_URI.abi --pkg=contracts --type=IERC1155Metadata_URI > ./contracts/IERC1155Metadata_URI.go
	abigen --abi=./contracts/abi/ISignatureValidator.abi --pkg=contracts --type=ISignatureValidator > ./contracts/ISignatureValidator.go
	abigen --abi=./contracts/abi/CryptopunksData.abi --pkg=contracts --type=CryptopunksData > ./contracts/CryptopunksData.go
	abigen --abi=./contracts/abi/Cryptopunks.abi --pkg=contracts --type=Cryptopunks > ./contracts/Cryptopunks.go
	abigen --abi=./contracts/abi/Zora.abi --pkg=contracts --type=Zora > ./contracts/Zora.go

g-docker:
	docker-compose down
	docker-compose build
	docker build -t bcgallery/gallery-postgres -f docker/postgres/DOCKERFILE .
	docker build -t bcgallery/gallery-postgres:circle -f docker/postgres/circle/DOCKERFILE .
	docker push bcgallery/gallery-postgres
	docker push bcgallery/gallery-postgres:circle
	docker-compose up -d

docker-start-clean:	docker-build
	docker-compose up -d

docker-build: docker-stop
	docker-compose build

docker-start: docker-stop
	docker-compose up -d

docker-stop: 
	docker-compose down

cloud-tasks:
	@cd ./cloud-tasks-emulator && go run ./ -port 8123 \
		-queue projects/gallery-local/locations/here/queues/feedbot \
		-queue projects/gallery-local/locations/here/queues/feed-event \
		-queue projects/gallery-local/locations/here/queues/token-processing \
		-queue projects/gallery-local/locations/here/queues/indexer-refreshes

migrate-coredb:
	migrate -path db/migrations/core -database "postgresql://postgres@localhost:5432/postgres?sslmode=disable" up

migrate-indexerdb:
	migrate -path db/migrations/indexer -database "postgresql://postgres@localhost:5433/postgres?sslmode=disable" up
