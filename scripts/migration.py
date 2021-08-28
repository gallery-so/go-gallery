import datetime
import csv
import requests
import hashlib
import time
import datetime
import os
import random
import threading
from pymongo import MongoClient


# hash function to create user id
def create_id():
    h = hashlib.md5()
    h.update(str(time.time()).encode("ascii"))
    return h.hexdigest()


########################################
# MAP EXISTING DATA TO MONGO DOCUMENTS #
########################################

# read in csv
# glry-users.csv
# username, profile_slug, wallet_address, email, created_at
# User schema
# version, id, creation_time, deleted, name, addresses, description, last_seen


# Initialize lists to hold all the documents that we create. These will be bulk inserted into Mongo at the end of the script
user_documents = []
collection_documents = []
gallery_documents = []
nft_documents = []
nonce_documents = []
errors = []

# Initialize a dictionary to keep track of collections. After we create empty collections for each user, we need to populate them with NFTs when we iterate through the NFT csv.
# Therefore, using a dictionary with the user_id as the key will make it efficient to populate the correct user's collection.
# The collections will be bulk inserted into Mongo at the end of the script.

# {
#    `user_id`: default_collection
#  }
user_collection_dict = {}

# dict to keep track of old id to new id
user_dict = {}


def create_nft(nft):
    try:
        print(nft["name"])
        if "rest" in nft:
            return
        if nft["contract_address"] == "" or nft["token_id"] == "":
            return

        supabase_user_id = nft["user_id"]

        user = user_dict[supabase_user_id]

        get_url = "https://api.opensea.io/api/v1/asset/{}/{}".format(
            nft["contract_address"], nft["token_id"]
        )
        print(get_url)
        r = requests.get(get_url, timeout=5)

        opensea_asset = r.json()

        nft_id = create_id()
        contract_document = {"contract_address": nft["contract_address"]}
        nft_document = {
            "version": 0,
            "_id": nft_id,
            "deleted": False,
            "name": nft["name"],
            "description": nft["description"],
            "external_url": nft["external_url"],
            "token_metadata_url": opensea_asset["token_metadata"],
            "creator_address": nft["creator_address"],
            "creator_name": nft["creator_opensea_name"],
            "owner_address": user["addresses"][0],
            "owner_user_id": user["_id"],
            "contract": contract_document,
            "opensea_id": opensea_asset["id"],
            "opensea_token_id": nft["token_id"],
            "image_url": opensea_asset["image_url"],
            "image_thumbnail_url": nft["image_thumbnail_url"],
            "image_preview_url": nft["image_preview_url"],
            "image_original_url": opensea_asset["image_original_url"],
            "animation_url": opensea_asset["animation_url"],
            "animation_original_url": opensea_asset["animation_original_url"],
        }

        nft_documents.append(nft_document)

        supabase_user_id = nft["user_id"]
        # only append nfts to the default collection if they are not hidden
        # all other nfts will be considered unassigned
        if not nft["hidden"]:
            user_collection_dict[supabase_user_id]["nfts"].append(nft_id)
    except Exception as e:
        errors.append(e)


with open("glry-users.csv", encoding="utf-8-sig") as usersfile:
    reader = csv.DictReader(usersfile)
    for user in reader:
        # load creation time as datetime
        print("USER", user["username"])
        creation_time_unix = datetime.datetime.strptime(
            user["created_at"], "%Y-%m-%dT%H:%M:%S.%fZ"
        ).timestamp()
        user_id = create_id()

        user_document = {
            "version": 0,
            "_id": user_id,
            "created_at": creation_time_unix,
            "last_updated": time.time_ns(),
            "deleted": False,
            "username": user["username"],
            "username_idempotent": user["username"].lower(),
            "addresses": [user["wallet_address"]],
        }

        nonce_document = {
            "version": 0,
            "_id": create_id(),
            "created_at": time.time_ns(),
            "last_updated": time.time_ns(),
            "deleted": False,
            "user_id": user_id,
            "address": user["wallet_address"],
            "value": str(random.randint(1000000000000000000, 9999999999999999999)),
        }

        default_col_id = create_id()

        # Since there is no concept of collections on the alpha, we will put all of a user's displayed NFTs in a default unnamed collection for v1.
        default_collection_document = {
            "version": 0,
            "_id": default_col_id,
            "creation_time": time.time_ns(),
            "last_updated": time.time_ns(),
            "deleted": False,
            "owner_user_id": user_id,
            "nfts": [],
            "hidden": False,
        }

        gallery_id = create_id()

        gallery_document = {
            "version": 0,
            "_id": gallery_id,
            "creation_time": time.time_ns(),
            "last_updated": time.time_ns(),
            "deleted": False,
            "owner_user_id": user_id,
            "collections": [default_col_id],
        }

        # Append the user and gallery documents to global list.
        user_documents.append(user_document)
        gallery_documents.append(gallery_document)
        nonce_documents.append(nonce_document)

        # Add the collection documents to the collection dictionary.
        # Use the supabase user id as the key instead of generated id, because the supabase user id is also available in the NFT csv, so it's easier to use.
        supabase_user_id = user["id"]
        user_dict[supabase_user_id] = user_document
        user_collection_dict[supabase_user_id] = default_collection_document


with open("glry-nfts.csv", encoding="utf-8-sig") as nftsFile:

    reader = csv.DictReader(
        nftsFile, restkey="rest", restval="", dialect=csv.unix_dialect
    )

    sorted_nfts = sorted(reader, key=lambda row: row["position"])
    threads = []
    for nft in sorted_nfts:
        t = threading.Thread(target=create_nft, args=(nft,))
        threads.append(t)
        t.start()

    # add all colls to collection_documents
    for thread in threads:
        thread.join()
    for coll in user_collection_dict.values():
        collection_documents.append(coll)
    for err in errors:
        print(err)


##############
# SAVE TO DB #
##############

mongo_url = os.environ["MONGO_URL"]

client = MongoClient(mongo_url)
db = client.gallery

# Select database collections (equivalent to tables)
user_collection = db.users
gallery_collection = db.galleries
collection_collection = db.collections
nft_collection = db.nfts
nonce_collection = db.nonces

# Bulk insert into database
user_collection.insert_many(user_documents)
gallery_collection.insert_many(gallery_documents)
collection_collection.insert_many(collection_documents)
nft_collection.insert_many(nft_documents)
nonce_collection.insert_many(nonce_documents)


# migration strategy
# for each existing user:
#   - create user and gallery documents.
#   - create 2 collection documents - default and hidden
# for each nft:
#   - create nft document
#   - populate user's collections, default or hidden, in the correct order
# save all documents in mongo

# version=0
