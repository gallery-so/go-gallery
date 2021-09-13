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
