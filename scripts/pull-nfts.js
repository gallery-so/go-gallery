const { createClient } = require("@supabase/supabase-js")
const fs = require("fs")

const supabaseUrl = "https://prcwfwuaxrvbkwblihbf.supabase.co"
const supabaseKey = process.env.SUPABASE_KEY
const supabase = createClient(supabaseUrl, supabaseKey)

const main = async () => {
  let { data: nfts } = await supabase
    .from("nfts")
    .select("*")
    .eq("hidden", false)
    .csv()

  fs.writeFile("glry-nfts.csv", nfts, err => {
    if (err) {
      throw err
    }
  })
}

main().catch(console.error)
