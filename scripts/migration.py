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
# version, id, creation_time, deleted, name, addresses, description, last_seen
with open('glry-users.csv') as usersfile:
    reader = csv.DictReader(usersfile)
    for row in reader:
        # load creation time as datetime
        print(row['username'])
        creation_time_unix = datetime.datetime.strptime(
            row['created_at'], '%Y-%m-%dT%H:%M:%S.%fZ').timestamp()
        user_id = create_user_id(row['wallet_address'])
        user_document = {
            "version": 0,
            "_id": user_id,
            "creation_time": creation_time_unix,
            "deleted": False,
            "name": row['username'],
            "addresses": [row['wallet_address']],
            "description": None,
            "last_seen": None
        }

        #TODO create 2 collections, default and hidden

        gallery_document = {
            'version': 0,
            '_id': # TODO create id the same way as done on backend
            'creation_time': creation_time_unix,
            'deleted': false,
            'owner_user_id': user_id,
            'collections': []
        }

        user2.insert_one(user_document)


# Collections
# version, id, creation_time, deleted, name, description, owner_user_id, nfts, hidden

# NFT
# version, id, creation_time, deleted, name, description, collection_names, external_url, creator_address, contract_address, opensea_id, opensea_token_id, image_url, image_thumbnail_url, image_preview_url, animation_url

with open('glry-nfts.csv') as nftsFile:
    reader = csv.DictReader(nftsFile)
    for row in reader:
        creation_time_unix = datetime.datetime.strptime(
            row['created_at'], '%Y-%m-%dT%H:%M:%S.%fZ').timestamp()
        contract_document = {
            contract_address: row['contract_address']
        }
        nft_document = {
            'version': 0,
            '_id': #TODO generate id same way as backend,
            'creation_time': creation_time_unix,
            'deleted': false,
            'name': row['name'],
            'description': row['description'],
            'collectors_note': null,
            'external_url': row['external_url'],
            'token_metadata_url': null, # Need to get from OpenSea
            'creator_address': row['creator_address'],
            'creator_name': null, # Need to get from OpenSea
            'owner_address': null, # TODO do we need this field? Should we have owner_id (GLRY user id instead?)
            'contract': contract_document, #TODO should this be a reference to the document instead?
            'opensea_id': null, # Need to get from OpenSea. but do we need? We only need contract address and token_id to query in OpenSea
            'opensea_token_id': row['token_id'],
            'image_url': null, # Need to get from OpenSea
            'image_thumbnail_url': row['image_thumbnail_url'],
            'image_preview_url': row['image_preview_url'],
            'image_original_url': null, # Need to get from OpenSea
            'animation_url': null, # Need to get from OpenSea
            'animation_original_url': null, # Need to get from OpenSea
            'acquisition_date': null, # Need to get from OpenSea
        }
        # TODO: Create 2 collections for each user. Default and Hidden. Use row['position'] to populate default collection, and row['hidden'] to determine which collection to put it in
        # 


# migration strategy
# for each existing user, create user and gallery docuemnts.

# version=0
