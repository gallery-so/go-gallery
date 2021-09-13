solc:
	solc --abi ./contracts/sol/IERC721.sol > ./contracts/abi/IERC721.abi
	solc --abi ./contracts/sol/IERC721Metadata.sol > ./contracts/abi/IERC721Metadata.abi
	solc --abi ./contracts/sol/IRedeemable.sol > ./contracts/abi/IRedeemable.abi
abi-gen:
	abigen --abi=./contracts/abi/IERC721.abi --pkg=contracts --type=IERC721 > ./contracts/IERC721.go
	abigen --abi=./contracts/abi/IERC721Metadata.abi --pkg=contracts --type=IERC721Metadata > ./contracts/IERC721Metadata.go
	abigen --abi=./contracts/abi/IRedeemable.abi --pkg=contracts --type=IRedeemable > ./contracts/IRedeemable.go