/**
 *   ____                  _                          _          ____        _
 *  / ___|_ __ _   _ _ __ | |_ ___  _ __  _   _ _ __ | | _____  |  _ \  __ _| |_ __ _
 * | |   | '__| | | | '_ \| __/ _ \| '_ \| | | | '_ \| |/ / __| | | | |/ _` | __/ _` |
 * | |___| |  | |_| | |_) | || (_) | |_) | |_| | | | |   <\__ \ | |_| | (_| | || (_| |
 *  \____|_|   \__, | .__/ \__\___/| .__/ \__,_|_| |_|_|\_\___/ |____/ \__,_|\__\__,_|
 *             |___/|_|            |_|
 *
 * On-chain Cryptopunk images and attributes, by Larva Labs.
 *
 * This contract holds the image and attribute data for the Cryptopunks on-chain.
 * The Cryptopunk images are available as raw RGBA pixels, or in SVG format.
 * The punk attributes are available as a comma-separated list.
 * Included in the attribute list is the head type (various color male and female heads,
 * plus the rare zombie, ape, and alien types).
 *
 * This contract was motivated by community members snowfro and 0xdeafbeef, including a proof-of-concept contract created by 0xdeafbeef.
 * Without their involvement, the project would not have come to fruition.
 */
interface CryptopunksData {
    /**
     * The Cryptopunk image for the given index.
     * The image is represented in a row-major byte array where each set of 4 bytes is a pixel in RGBA format.
     * @param index the punk index, 0 <= index < 10000
     */
    function punkImage(uint16 index) external view returns (bytes memory);

    /**
     * The Cryptopunk image for the given index, in SVG format.
     * In the SVG, each "pixel" is represented as a 1x1 rectangle.
     * @param index the punk index, 0 <= index < 10000
     */
    function punkImageSvg(uint16 index)
        external
        view
        returns (string memory svg);

    /**
     * The Cryptopunk attributes for the given index.
     * The attributes are a comma-separated list in UTF-8 string format.
     * The first entry listed is not technically an attribute, but the "head type" of the Cryptopunk.
     * @param index the punk index, 0 <= index < 10000
     */
    function punkAttributes(uint16 index)
        external
        view
        returns (string memory text);
}
