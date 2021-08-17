import datetime
import csv
import json
import hashlib
import time
import datetime
from pymongo import MongoClient

client = MongoClient(
    "mongodb+srv://mike:<w6jhy5oivdMCVqzC>@cluster0.p9jwh.mongodb.net/test?retryWrites=true&w=majority")
db = client.test

# what are the collections i need?
user2 = db.user2


# hash function to create user id
def create_user_id(wallet_address):
    h = hashlib.md5()
    h.update(str(time.time()).encode('ascii'))
    h.update(wallet_address.encode('ascii'))
    return h.hexdigest()


# read in csv
# glry-users.csv
# username, profile_slug, wallet_address, email, created_at
# User schema
# version, id, creation_time, deleted, name, addresses, bio, last_seen
with open('glry-users.csv') as usersfile:
    reader = csv.DictReader(usersfile)
    for row in reader:
        # load creation time as datetime
        print(row['username'])
        creation_time_unix = datetime.datetime.strptime(
            row['created_at'], '%Y-%m-%dT%H:%M:%S.%fZ').timestamp()
        user_document = {
            "version": 0,
            "_id": create_user_id(row['wallet_address']),
            "creation_time": creation_time_unix,
            "deleted": False,
            "name": row['username'],
            "addresses": [row['wallet_address']],
            "bio": None,
            "last_seen": None
        }
        user2.insert_one(user_document)


# Collections
# version, id, creation_time, deleted, name, bio, owner_user_id, nfts, hidden

# NFT
# version, id, creation_time, deleted, name, bio, collection_names, external_url, creator_address, contract_address, opensea_id, opensea_token_id, image_url, image_thumbnail_url, image_preview_url, animation_url

# version=0
