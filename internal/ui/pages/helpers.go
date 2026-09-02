package pages

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func totalPages(total, pageSize int) int {
	if pageSize <= 0 {
		return 1
	}
	pages := (total + pageSize - 1) / pageSize
	if pages < 1 {
		return 1
	}
	return pages
}

func calculateAdminPageNumbers(current, total int) []int {
	if total <= 1 {
		return []int{1}
	}
	start := current - 2
	if start < 1 {
		start = 1
	}
	end := start + 4
	if end > total {
		end = total
		start = end - 4
		if start < 1 {
			start = 1
		}
	}
	var nums []int
	for i := start; i <= end; i++ {
		nums = append(nums, i)
	}
	return nums
}
