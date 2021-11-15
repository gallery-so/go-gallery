import requests
from pymongo import MongoClient

client = MongoClient("")
db = client.gallery

collection_collection = db.collections
nft_collection = db.nfts
gallery_collection = db.galleries
user_collection = db.users


def dif(r1, r2):
    return set(r1) - set(r2)


def identifiersv1(nft):
    return "{}-{}".format(
        nft["asset_contract"]["address"].lower(), int(nft["opensea_token_id"].lower())
    )


def identifiersv2(nft):
    return "{}-{}".format(nft["contract_address"].lower(), int(nft["token_id"].lower()))


for user in user_collection.find():
    try:
        print(user["username"], ":")
        urlv1 = "https://api.dev.gallery.so/glry/v1/nfts/user_get?user_id={}".format(
            user["_id"]
        )
        urlv2 = "https://api.dev.gallery.so/glry/v2/nfts/user_get?user_id={}&limit=5000".format(
            user["_id"]
        )

        r1 = requests.get(urlv1, timeout=10).json()
        r2 = requests.get(urlv2, timeout=10).json()

        print("V1 Len", len(r1["nfts"]))
        print("V2 Len", len(r2["nfts"]))

        print(
            "Diff V1",
            dif(map(identifiersv1, r1["nfts"]), map(identifiersv2, r2["nfts"])),
        )
        print(
            "Diff V2",
            dif(map(identifiersv2, r2["nfts"]), map(identifiersv1, r1["nfts"])),
        )
        print("=============================================")

    except Exception as e:
        print(e)
