begin;

-- Static table lookup for marketplace contracts such as OpenSea and Zora
create table if not exists marketplace_contracts (
  contract_id varchar(255) primary key references contracts(id)
);

insert into marketplace_contracts (
  select
    unnest(array[
      '2EpXhbM5HiZ52xwn0rJXwiq9fsD', -- hic et nunc NFTs
      '249yE25jK2ZykbFb2ir5EHE8n9w', -- OpenSea Shared Storefront
      '23mvT8uANHkU4mHUjTn15sEAwjt', -- ENS
      '23zWlMDCH9L9EoVbgM2QdLw2nL6', -- Rarible
      '23WGw0PW3XeXABPE2kmbiIQ5pyi', -- SuperRare
      '24ACDJE4pmskAYaFwfJSSErXyDA', -- Zora
      '24AKCslsotgwERjpEu3Ho8Oi0da', -- Foundation
      '2EpXhcso7lhUxS6POWAQka1Tmlj', -- Versum Items
      '24EA0Wbwu5mQSw0xFV0cWspftx4', -- KnownOriginDigitalAsset
      '23L8QCst9dmAZt4FusArHPHHvYw' -- KnownOriginDigitalAsset
])) on conflict do nothing;

commit;
