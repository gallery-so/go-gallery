// SPDX-License-Identifier: MIT

pragma solidity ^0.8.0;

abstract contract Redeemable {
    event Redeem(address indexed redeemer, uint256 indexed tokenID);

    mapping(uint256 => address) private _redeemed;
    mapping(address => bool) private _hasRedeemed;

    function isRedeemed(uint256 _tokenID) public view returns (bool) {
        return _redeemed[_tokenID] != address(0);
    }

    function isRedeemedBy(uint256 tokenID, address redeemer)
        public
        view
        returns (bool)
    {
        return _redeemed[tokenID] == redeemer;
    }

    function redeem(uint256 _tokenID) public virtual {
        require(_redeemed[_tokenID] == address(0));
        require(!_hasRedeemed[msg.sender]);
        _redeemed[_tokenID] = msg.sender;
        _hasRedeemed[msg.sender] = true;
        emit Redeem(msg.sender, _tokenID);
    }
}
