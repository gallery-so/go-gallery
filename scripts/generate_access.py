from pymongo import MongoClient
import requests

client = MongoClient()
db = client.gallery


user_collection = db.users
access_collection = db.access


for user in user_collection.find():
    try:
        if access_collection.find_one({"user_id": user["_id"]}) is None:
            requests.get(
                "https://api.dev.gallery.so/features/v1/access/user_get?user_id={}".format(
                    user["_id"]
                )
            )
    except Exception as e:
        print(e)
