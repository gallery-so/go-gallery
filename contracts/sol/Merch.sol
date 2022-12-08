interface Merch {
    function redeemAdmin(uint256[] calldata tokenIDs) external;

    function tokenURI(uint256 tokenId) external view returns (string memory);

    function ownerOf(uint256 tokenId) external view returns (address owner);

    function isRedeemed(uint256 tokenId) external view returns (bool);
}
