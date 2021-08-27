import datetime
import csv
import requests
import hashlib
import time
import datetime
import os
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

# Initialize a dictionary to keep track of collections. After we create empty collections for each user, we need to populate them with NFTs when we iterate through the NFT csv.
# Therefore, using a dictionary with the user_id as the key will make it efficient to populate the correct user's collection.
# The collections will be bulk inserted into Mongo at the end of the script.

# {
#    `user_id`: default_collection
#  }
user_collection_dict = {}

# dict to keep track of old id to new id
user_dict = {}


with open("glry-users.csv") as usersfile:
    reader = csv.DictReader(usersfile)
    for user in reader:
        # load creation time as datetime
        print(user["username"])
        creation_time_unix = datetime.datetime.strptime(
            user["created_at"], "%Y-%m-%dT%H:%M:%S.%fZ"
        ).timestamp()
        user_id = create_id()

        user_document = {
            "version": 0,
            "_id": user_id,
            "creation_time": creation_time_unix,
            "deleted": False,
            "name": user["username"],
            "addresses": [user["wallet_address"]],
        }

        default_col_id = create_id()

        # Since there is no concept of collections on the alpha, we will put all of a user's displayed NFTs in a default unnamed collection for v1.
        default_collection_document = {
            "version": 0,
            "_id": default_col_id,
            "creation_time": time.time_ns(),
            "deleted": False,
            "owner_user_id": user_id,
            "nfts": [],
            "hidden": False,
        }

        gallery_id = create_id()

        gallery_document = {
            "version": 0,
            "_id": gallery_id,  # TODO create id the same way as done on backend
            "creation_time": time.time_ns(),
            "deleted": False,
            "owner_user_id": user_id,
            "collections": [default_col_id],
        }

        # Append the user and gallery documents to global list.
        user_documents.append(user_document)
        gallery_documents.append(gallery_document)

        # Add the collection documents to the collection dictionary.
        # Use the supabase user id as the key instead of generated id, because the supabase user id is also available in the NFT csv, so it's easier to use.
        supabase_user_id = user["id"]
        user_dict[supabase_user_id] = user_document
        user_collection_dict[supabase_user_id] = default_collection_document


with open("glry-nfts.csv") as nftsFile:
    reader = csv.DictReader(nftsFile)
    # Sort the data by user_id and position. That way all nfts are already in the correct order so we can insert them into the Collections documents without accounting for position
    sortedNfts = sorted(reader, key=lambda row: (row["user_id"], row["position"]))
    for nft in sortedNfts:

        creation_time_unix = datetime.datetime.strptime(
            nft["created_at"], "%Y-%m-%dT%H:%M:%S.%fZ"
        ).timestamp()

        supabase_user_id = nft["user_id"]
        user = user_dict[supabase_user_id]

        r = requests.get(
            "https://api.opensea.io/api/v1/asset/{}/{}".format(
                nft["contract_address"], nft["token_id"]
            )
        )

        opensea_asset = r.json()

        nft_id = create_id()
        contract_document = {"contract_address": nft["contract_address"]}
        nft_document = {
            "version": 0,
            "_id": nft_id,
            "creation_time": creation_time_unix,
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
        coll = user_collection_dict[supabase_user_id]
        # only append nfts to the default collection if they are not hidden
        # all other nfts will be considered unassigned
        if not nft["hidden"]:
            coll.nfts.append(nft_id)
    # add all colls to collection_documents
    for coll in user_collection_dict:
        collection_documents.append(coll)


##############
# SAVE TO DB #
##############

mongo_url = os.environ["MONGO_URL"]

client = MongoClient(mongo_url)
db = client.gallery

# Select database collections (equivalent to tables)
userCollection = db.users
galleryCollection = db.galleries
collectionCollection = db.collections
nftCollection = db.nfts

# Bulk insert into database
userCollection.insert_many(user_documents)
galleryCollection.insert_many(gallery_documents)
collectionCollection.insert_many(collection_documents)
nftCollection.insert_many(nft_documents)


# migration strategy
# for each existing user:
#   - create user and gallery documents.
#   - create 2 collection documents - default and hidden
# for each nft:
#   - create nft document
#   - populate user's collections, default or hidden, in the correct order
# save all documents in mongo

# version=0
