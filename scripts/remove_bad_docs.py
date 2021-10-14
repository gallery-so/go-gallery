from pymongo import MongoClient


client = MongoClient("")
db = client.gallery

collection_collection = db.collections
nft_collection = db.nfts

for nft in nft_collection.find({"owner_address": ""}):
    print("NONE NFT", nft["_id"])
    nft_collection.delete_one({"_id": nft["_id"]})

for nft in nft_collection.find():
    if not nft["owner_address"].islower():
        print("LOWER", nft["_id"])
        nft_collection.delete_one({"_id": nft["_id"]})

for coll in collection_collection.find({"nfts": []}):
    print("NONE COLL", coll["_id"])
    collection_collection.delete_one({"_id": coll["_id"]})
