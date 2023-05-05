import csv
from pymongo import MongoClient

client = MongoClient()
db = client.gallery

user_collection = db.users

wallet_count_greater = 0
wallet_count_to_amount = {}
while True:
    count = user_collection.count_documents(
        {"addresses.{}".format(wallet_count_greater): {"$exists": True}}
    )
    amount = count
    if amount == 0:
        break
    wallet_count_to_amount[wallet_count_greater] = amount
    wallet_count_greater += 1


total = 0
for i in range(wallet_count_greater):
    back = wallet_count_greater - i - 1
    wallet_count_to_amount[back] = wallet_count_to_amount[back] - total
    total += wallet_count_to_amount[back]

for k, v in wallet_count_to_amount.items():
    print(f"{k + 1} wallets: {v}")
    print(f"{k + 1} ratio: {v / total * 100}%")
