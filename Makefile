solc:
	solc --abi ./contracts/sol/IERC721.sol > ./contracts/abi/IERC721.json
	solc --abi ./contracts/sol/IENS.sol > ./contracts/abi/IENS.json
abi-gen:
	abigen --abi=./contracts/abi/IERC721.json --pkg=contracts --type=IERC721 > ./contracts/IERC721.go
	abigen --abi=./contracts/abi/IENS.json --pkg=contracts --type=IENS > ./contracts/IENS.go