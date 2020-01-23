package storage

func newFromFS(fs fs) extendedPriceModel { // TODO return error
	memstore := &memStore{}
	batchWriter := &batchWriter{fs: fs}
	var previousPrices priceReader = memstore // TODO null store?

	// TODO check link count consistency
	// TODO check nRrecords fields and rename appropriately
	// TODO entrySeq's for product directories? do lazily...

	// restore sequence numbers from results directory
	files, err := fs.Sub(ResultsSubdirectory).Files()
	if err != nil {
		panic(err)
	}
	if len(files) > 0 {
		var f filename
		err := f.FromString(files[len(files)-1])
		if err != nil {
			panic(err)
		}

		previousPrices = priceLoader{
			snapshotFS{
				bound:  filename{fileSeq: f.fileSeq + 1}.String(),
				readFS: fs,
			},
		}

		batchWriter.fileSeq = f.fileSeq
		batchWriter.entrySeq = f.entrySeq
	}

	// TODO plumb errors, context
	return extendModel{
		priceModel:        linearizeUpdates(memstore, previousPrices, batchWriter),
		priceLogRetriever: priceLoader{fs},
	}
}

// FIXME refactor, used to decorate priceModel with additional log fetching API,
// still not sure if all model implementations should support that
type extendModel struct {
	priceModel
	priceLogRetriever
}

var _ extendedPriceModel = extendModel{}
