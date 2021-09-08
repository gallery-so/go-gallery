solc:
	solc --abi ./contracts/sol/IERC721.sol > ./contracts/abi/IERC721.json
	solc --abi ./contracts/sol/IRedeemable.sol > ./contracts/abi/IRedeemable.json
abi-gen:
	abigen --abi=./contracts/abi/IERC721.json --pkg=contracts --type=IERC721 > ./contracts/IERC721.go
	abigen --abi=./contracts/abi/IRedeemable.json --pkg=contracts --type=IRedeemable > ./contracts/IRedeemable.go