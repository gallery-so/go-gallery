solc:
<<<<<<< HEAD
	solc --abi ./contracts/sol/IERC721.sol > ./contracts/abi/IERC721.abi
	solc --abi ./contracts/sol/IERC721Metadata.sol > ./contracts/abi/IERC721Metadata.abi
	solc --abi ./contracts/sol/IERC1155.sol > ./contracts/abi/IERC1155.abi
	solc --abi ./contracts/sol/IENS.sol > ./contracts/abi/IENS.abi
	solc --abi ./contracts/sol/IERC1155Metadata_URI.sol > ./contracts/abi/IERC1155Metadata_URI.abi
abi-gen:
	abigen --abi=./contracts/abi/IERC721.abi --pkg=contracts --type=IERC721 > ./contracts/IERC721.go
	abigen --abi=./contracts/abi/IERC721Metadata.abi --pkg=contracts --type=IERC721Metadata > ./contracts/IERC721Metadata.go
	abigen --abi=./contracts/abi/IERC1155.abi --pkg=contracts --type=IERC1155 > ./contracts/IERC1155.go
	abigen --abi=./contracts/abi/IENS.abi --pkg=contracts --type=IENS > ./contracts/IENS.go
	abigen --abi=./contracts/abi/IERC1155Metadata_URI.abi --pkg=contracts --type=IERC1155Metadata_URI > ./contracts/IERC1155Metadata_URI.go
=======
	solc --abi ./contracts/sol/IENS.sol > ./contracts/abi/IENS.abi
	solc --abi ./contracts/sol/IERC1155.sol > ./contracts/abi/IERC1155.abi
abi-gen:
	abigen --abi=./contracts/abi/IENS.abi --pkg=contracts --type=IENS > ./contracts/IENS.go
	abigen --abi=./contracts/abi/IERC1155.abi --pkg=contracts --type=IERC1155 > ./contracts/IERC1155.go
>>>>>>> main
