diff:
	@git show origin/main:_encrypted_local | keybase decrypt > ../_originlocal.zip
	@unzip -qq ../_originlocal.zip -d ../_originlocal
	@diff -u -r ../_originlocal . || rm -rf ../_originlocal ../_originlocal.zip

confirm:
	@prompt=$(shell bash -c 'read -p "Does this diff look correct? [y]: " prompt; echo $$prompt'); \
	if [ "$$prompt" != "y" ]; then exit 1; fi

encrypt: diff confirm
	zip -r ../_local.zip ./
	keybase encrypt --team usegallery.internal -i ../_local.zip -o ../_encrypted_local
	rm ../_local.zip

decrypt:
	keybase decrypt -i ../_encrypted_local -o ../_local.zip
	unzip -o ../_local.zip
	rm ../_local.zip
	chmod 600 ./postgres-certs/**/*.cer
