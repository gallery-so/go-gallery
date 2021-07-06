import datetime
import csv
import json
import hashlib
import time
import datetime
from pymongo import MongoClient



# hash function to create user id
def create_user_id(wallet_address):
    h = hashlib.md5()
    h.update(str(time.time()).encode('ascii'))
    h.update(wallet_address.encode('ascii'))
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
#    `user_id`: {
#        default_collection: {} <- collection document representing visible NFTs
#        hidden_collection: {} <- <- collection document representing hidden NFTs
#    }
#  }
user_collection_dict = {}

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

        # Since there is no concept of collections on the alpha, we will put all of a user's displayed NFTs in a default unnamed collection for v1.
        default_collection_document = {
            'version': 0,
            '_id': 0, # TODO change
            'creation_time': creation_time_unix, # Is it fine if creation_time for all migrated data is just the original user's created_time?
            'deleted': false,
            'name': null, # Default gallery will not have a name
            'collectors_note': null, # Default gallery will not have a collectors note
            'owner_user_id': user_id,
            'nfts': [],
            'hidden': false
        }

        # NFTs marked as hidden in the alpha will be put in the "hidden" collection representing unassigned NFTs.
        hidden_collection_document = {
            'version': 0,
            '_id': 0, # TODO change
            'creation_time': creation_time_unix, # Is it fine if creation_time for all migrated data is just the original user's created_time?
            'deleted': false,
            'name': 'GLRY__RESERVED__UNASSIGNED', # TODO change name to reserved name
            'collectors_note': null, # Hidden collection doesnt need collectors note
            'owner_user_id': user_id,
            'nfts': [],
            'hidden': true
        }

        gallery_document = {
            'version': 0,
            '_id':  0, # TODO create id the same way as done on backend
            'creation_time': creation_time_unix,
            'deleted': false,
            'owner_user_id': user_id,
            'collections': []
        }


        # Append the user and gallery documents to global list.
        user_documents.append(user_document)
        gallery_documents.append(gallery_document)

        # Add the collection documents to the collection dictionary.
        # Use the supabase user id as the key instead of generated id, because the supabase user id is also available in the NFT csv, so it's easier to use.
        user_id = row['id']
        user_collection_dict[user_id]['default_collection'] = default_collection_document
        user_collection_dict[user_id]['hidden_collection'] = hidden_collection_document



with open('glry-nfts.csv') as nftsFile:
    reader = csv.DictReader(nftsFile)
    # Sort the data by user_id and position. That way all nfts are already in the correct order so we can insert them into the Collections documents without accounting for position
    sortedNfts = sorted(reader, key=lambda row:(row['user_id'], row['position']))
    for row in sortedNfts:
        creation_time_unix = datetime.datetime.strptime(
            row['created_at'], '%Y-%m-%dT%H:%M:%S.%fZ').timestamp()
            
        global contract_document
        global nft_document

        contract_document = {
            contract_address: row['contract_address']
        }
        nft_document = {
            'version': 0,
            '_id':  0, #TODO generate id same way as backend,
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

        nft_documents.append(nft_document)
        
        # TODO: Populate collections with nft. Default and Hidden. Use row['position'] to populate default collection, and row['hidden'] to determine which collection to put it in
        # For each nft, access the user's collections in the dictionary
        # If not hidden, update default collection
        # If hidden, update hidden collection
        # Order: csv has already been sorted
        user_id = row['user_id']
        if row['hidden']:
            user_collection_dict[user_id]['hidden_collection'].nfts.append(nft_document) # TODO do we append the document or the id?
        else: 
            user_collection_dict[user_id]['default_collection'].nfts.append(nft_document) # TODO do we append the document or the id?


##############
# SAVE TO DB #
##############

# client = MongoClient(
#     "mongodb+srv://mike:<w6jhy5oivdMCVqzC>@cluster0.p9jwh.mongodb.net/test?retryWrites=true&w=majority")
# testDb = client.test

# # Select database collections (equivalent to tables)
# userCollection = testDb.user2 # Why is this named user2?
# galleryCollection = testDb.gallery
# collectionCollection = testDb.collection
# nftCollection = testDb.nft

# # Bulk insert into database
# userCollection.bulk_write(user_documents)
# galleryCollection.bulk_write(gallery_documents)
# collectionCollection.bulk_write(collection_documents)
# nftCollection.bulk_write(nft_documents)


# migration strategy
# for each existing user:
#   - create user and gallery documents.
#   - create 2 collection documents - default and hidden
# for each nft:
#   - create nft document
#   - populate user's collections, default or hidden, in the correct order
# save all documents in mongo

# version=0
