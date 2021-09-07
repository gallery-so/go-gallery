solc:
	solc --abi ./contracts/sol/IERC721.sol > ./contracts/abi/IERC721.json
	solc --abi ./contracts/sol/Redeemable.sol > ./contracts/abi/Redeemable.json
abi-gen:
	abigen --abi=./contracts/abi/IERC721.json --pkg=contracts --type=IERC721 > ./contracts/IERC721.go
	abigen --abi=./contracts/abi/Redeemable.json --pkg=contracts --type=Redeemable > ./contracts/Redeemable.go