const { createClient } = require("@supabase/supabase-js")
const fs = require("fs")
const { parse } = require("json2csv")

const supabaseUrl = "https://prcwfwuaxrvbkwblihbf.supabase.co"
const supabaseKey = process.env.SUPABASE_KEY
const supabase = createClient(supabaseUrl, supabaseKey)

const fields = [
  "id",
  "user_id",
  "animation_url",
  "image_url",
  "description",
  "name",
  "owner_opensea_username",
  "collection_name",
  "position",
  "external_url",
  "creator_opensea_name",
  "created_date",
  "creator_address",
  "contract_address",
  "token_id",
  "hidden",
  "opensea_url",
  "image_preview_url",
  "image_thumbnail_url",
]
const opts = { fields }
const main = async () => {
  let allNFTs = []
  for (let i = 0; i < 50000; i += 10000) {
    let { data: nfts } = await supabase
      .from("nfts")
      .select("*")
      .eq("hidden", false)
      .order("id", { ascending: false })
      .range(i, i + 9999)

    // console.log(nfts.length)
    allNFTs.push(...nfts)
  }
  console.log(allNFTs.length)
  allNFTs = uniqBy(allNFTs, nft => nft.id)
  console.log(allNFTs.length)

  const csv = parse(allNFTs, opts)

  fs.writeFile("glry-nfts.csv", csv, err => {
    if (err) {
      console.error(err)
      throw err
    }
  })

  let { data: users } = await supabase
    .from("users")
    .select("*")
    .neq("username", "")
    .csv()

  fs.writeFile("glry-users.csv", users, err => {
    if (err) {
      console.error(err)
      throw err
    }
  })
}

main().catch(console.error)

function uniqBy(a, key) {
  var seen = {}
  return a.filter(function (item) {
    var k = key(item)
    if (seen.hasOwnProperty(k)) {
      console.log(k)
    }
    return seen.hasOwnProperty(k) ? false : (seen[k] = true)
  })
}
