package indexing

func SetLabelledClientsLimits(page_size, max_terms int) func() {
	old_page_size, old_max_terms := labelledClientsPageSize, maxTermsCount
	labelledClientsPageSize, maxTermsCount = page_size, max_terms
	return func() {
		labelledClientsPageSize, maxTermsCount = old_page_size, old_max_terms
	}
}
