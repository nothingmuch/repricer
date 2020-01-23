package storage

func newFromFS(fs fs) priceModel { // TODO return error

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
	return linearizeUpdates(memstore, previousPrices, batchWriter)
}
