from pymongo import MongoClient

client = MongoClient()
db = client.gallery

token_collection = db.tokens

count = token_collection.find_one({})

print(count)
