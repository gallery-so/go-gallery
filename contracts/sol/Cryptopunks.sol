interface Cryptopunks {
    function punkIndexToAddress(uint256 _punkIndex)
        external
        view
        returns (address);
}
