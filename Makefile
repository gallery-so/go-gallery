abi-gen:
	solc --abi ./contracts/IERC721.sol > ./contracts/IERC721.json
	solc --bin ./contracts/IERC721.sol > ./contracts/IERC721.bin 
	abigen --bin=./contracts/IERC721.bin --abi=./contracts/IERC721.json --pkg=contracts > ./contracts/IERC721.go
	echo "Most likely the generated go file will need some cleanup"