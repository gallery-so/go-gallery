interface Merch {
    function redeem(uint256[] calldata tokenIDs) external;

    function tokenURI(uint256 tokenId) external view returns (string memory);

    function isRedeemed(uint256 tokenId) external view returns (bool);
}
