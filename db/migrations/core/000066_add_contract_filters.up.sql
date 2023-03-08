-- Static table lookup for marketplace contracts such as OpenSea and Zora
create table if not exists marketplace_contracts (
  contract_id varchar(255) primary key references contracts(id)
);
