import csv
from pymongo import MongoClient

client = MongoClient()
db = client.gallery

collection_collection = db.collections
nft_collection = db.nfts
gallery_collection = db.galleries
user_collection = db.users

with open("users.csv", "w", newline="") as csvfile:
    fieldnames = ["username", "url", "nfts", "collections"]
    writer = csv.DictWriter(csvfile, fieldnames=fieldnames)

    writer.writeheader()
    for user in user_collection.find():
        try:
            username = user["username"]
            url = "https://gallery.so/{}".format(user["username_idempotent"])
            collections = collection_collection.count_documents(
                {"owner_user_id": user["_id"]}
            )
            nfts = 0
            for address in user["addresses"]:
                nfts += nft_collection.count_documents({"owner_address": address})
            row = {
                "username": username,
                "url": url,
                "nfts": nfts,
                "collections": collections,
            }
            print(row)
            writer.writerow(row)
        except Exception as e:
            print(e)
