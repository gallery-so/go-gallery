package glry_lib

import (
	"context"

	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
)

//-------------------------------------------------------------
// INPUT
type GLRYcollGetInput struct {
	UserIDstr glry_db.GLRYuserID `validate:"required,min=4,max=50"`
}

// OUTPUT
type GLRYcollGetOutput struct {
	CollsOutputsLst []map[string]interface{}
}

// INPUT
type GLRYcollCreateInput struct {
	OwnerUserIdStr    string `json:"user_id" validate:"required"`
	NameStr           string `json:"name"        validate:"required,min=4,max=50"`
	CollectorsNoteStr string `json:"collectors_note" validate:"required,min=0,max=500"`
}

// OUTPUT
type GLRYcollCreateOutput struct {
	IDstr   glry_db.GLRYcollID `json:"id"`
	NameStr string             `json:"name"`
}

// INPUT
// FIX!! - currently coll IDs are mongodb ID's,
//         have some mongodb agnostic ID format.
type GLRYcollDeleteInput struct {
	IDstr string `json:"id" validate:"required,len=24"`
}

// OUTPUT
type GLRYcollDeleteOutput struct {
}

//-------------------------------------------------------------
func CollGetPipeline(pInput *GLRYcollGetInput,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) (*GLRYcollGetOutput, error) {

	collsLst, err := glry_db.CollGetByUserID(pInput.UserIDstr,
		pCtx,
		pRuntime)
	if err != nil {
		return nil, err
	}

	collsOutputsLst := []map[string]interface{}{}
	for _, coll := range collsLst {

		/*
			COLL_OUTPUT:
			{
				id: 1,
				isHidden: true,
				name: 'Cool Collection',
				description: 'my favorites',
				// ! note: we want the CREATOR opensea username, not OWNER username
				nfts: [ { id: 1, name: 'cool nft', creator_username_opensea: 'ColorGlyphs' }, {}, {}, ...]
			},
		*/
		collOutputMap := map[string]interface{}{
			"id":              coll.IDstr,
			"hidden":          coll.HiddenBool,
			"name":            coll.NameStr,
			"collectors_note": coll.CollectorsNoteStr,
			"nfts":            []map[string]interface{}{},
		}

		collsOutputsLst = append(collsOutputsLst, collOutputMap)
	}

	output := &GLRYcollGetOutput{
		CollsOutputsLst: collsOutputsLst,
	}

	return output, nil
}

//-------------------------------------------------------------
// CREATE
func CollCreatePipeline(pInput *GLRYcollCreateInput,
	pUserIDstr string,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) (*GLRYcollCreateOutput, error) {

	err := glry_core.Validate(pInput, pRuntime)
	if err != nil {
		return nil, err
	}

	//------------------

	nameStr := pInput.NameStr
	ownerUserIDstr := pUserIDstr

	coll := &glry_db.GLRYcollectionStorage{
		NameStr:           nameStr,
		CollectorsNoteStr: pInput.CollectorsNoteStr,
		OwnerUserIDstr:    ownerUserIDstr,
		DeletedBool:       false,
		NFTsLst:           []string{},
	}

	// DB
	err = glry_db.CollCreate(coll, pCtx, pRuntime)
	if err != nil {
		return nil, err
	}

	output := &GLRYcollCreateOutput{
		IDstr:   coll.IDstr,
		NameStr: coll.NameStr,
	}

	return output, nil
}

//-------------------------------------------------------------
// DELETE
func CollDeletePipeline(pInput *GLRYcollDeleteInput,
	pCtx context.Context,
	pRuntime *glry_core.Runtime) (*GLRYcollDeleteOutput, error) {

	//------------------
	// VALIDATE
	err := glry_core.Validate(pInput, pRuntime)
	if err != nil {
		return nil, err
	}

	//------------------

	return nil, nil
}

//-------------------------------------------------------------
