const { createClient } = require("@supabase/supabase-js")
const fs = require("fs")
const { parse } = require("json2csv")

const supabaseUrl = "https://prcwfwuaxrvbkwblihbf.supabase.co"
const supabaseKey = process.env.SUPABASE_KEY
const supabase = createClient(supabaseUrl, supabaseKey)

const main = async () => {
  let { data: nfts, error } = await supabase.from("nfts").select("*")
  if (error) {
    throw error
    return
  }

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
    "rank",
  ]
  const opts = { fields }
  const csv = parse(nfts, opts)
  fs.writeFile("glry-nfts.csv", csv, err => {
    if (err) {
      throw err
      return
    }
  })
}

main().catch(console.error)
