alter table token_definitions add column if not exists is_fxhash boolean not null default false;

update token_definitions set is_fxhash = true
from (
	select td.id
	from token_definitions td
	join contracts c on td.contract_id = c.id
	where 
	-- tezos fxhash contracts
	(c.chain = 4 and c.address in (
		'KT1KEa8z6vWXDJrVqtMrAeDVzsvxat3kHaCE',
		'KT1U6EHmNxJTkvaWJ4ThczG4FSDaHC21ssvi',
		'KT1EfsNuqwLAWDd3o4pvfUx1CAh5GMdTrRvr',
		'KT1GtbuswcNMGhHF2TSuH1Yfaqn16do8Qtva',
		'KT1RJ6PbjHpwc3M5rw5s2Nbmefwbuwbdxton',
		'KT19xbD2xn6A81an18S35oKtnkFNr9CVwY5m'
	))
	or
	-- eth fxhash contracts
	(c.chain = 0 and c.symbol = 'FXGEN')
) fxhash_tokens
where fxhash_tokens.id = token_definitions.id;
