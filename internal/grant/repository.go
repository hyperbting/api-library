package grant

import (
	"cloud.google.com/go/firestore"
)

// DocRef is the user's grants document. Other packages (save, redeem) run their transactions over it,
// so a claim or a new grant commits together with their own document.
func DocRef(client *firestore.Client, uid string) *firestore.DocumentRef {
	return client.Collection(UsersCollection).Doc(uid).Collection(PrivateCollection).Doc(GrantsDocID)
}

// Decode reads a grants document snapshot; a missing document (snap == nil or not Exists) gives a fresh Doc.
func Decode(snap *firestore.DocumentSnapshot) (*Doc, error) {
	if snap == nil || !snap.Exists() {
		return NewDoc(), nil
	}
	var doc Doc
	if err := snap.DataTo(&doc); err != nil {
		return nil, err
	}
	if doc.Grants == nil {
		doc.Grants = map[string]*Grant{}
	}
	return &doc, nil
}
