interface Zora {
    function tokenMetadataURI(uint256 tokenId)
        external
        view
        returns (string memory);

    function tokenURI(uint256 tokenId) external view returns (string memory);
}
