solc:
	solc --abi ./contracts/sol/IENS.sol > ./contracts/abi/IENS.abi
	solc --abi ./contracts/sol/IERC1155.sol > ./contracts/abi/IERC1155.abi
abi-gen:
	abigen --abi=./contracts/abi/IENS.abi --pkg=contracts --type=IENS > ./contracts/IENS.go
	abigen --abi=./contracts/abi/IERC1155.abi --pkg=contracts --type=IERC1155 > ./contracts/IERC1155.go