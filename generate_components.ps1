$dir = "f:/Dawa 24/dawa24-store/internal/ui/components"
New-Item -ItemType Directory -Force -Path $dir | Out-Null

$files = @{
    "datatable.templ" = @"
package components

type Column struct {
	Key      string
	Label    string
	Sortable bool
}

type DataTableProps struct {
	Columns []Column
	State   string // "loading", "empty", "error", "ready", "partial"
	Error   string
}

templ DataTable(props DataTableProps) {
	<div class="table-container overflow-auto">
		if props.State == "loading" {
			@SkeletonTable()
		} else if props.State == "error" {
			@EmptyState(EmptyStateProps{Title: "Error", Message: props.Error, ActionLabel: "Retry"})
		} else if props.State == "empty" {
			@EmptyState(EmptyStateProps{Title: "No Data", Message: "No records found.", ActionLabel: "Add New"})
		} else {
			<table class="table min-w-full">
				<thead class="sticky top-0 bg-white shadow-sm">
					<tr>
						for _, col := range props.Columns {
							<th class="px-6 py-3 border-b text-left text-xs font-medium text-gray-500 uppercase tracking-wider" aria-sort={ func() string { if col.Sortable { return "none" } return "" }() }>
								{ col.Label }
							</th>
						}
						<th>Actions</th>
					</tr>
				</thead>
				<tbody>
					{ children... }
				</tbody>
			</table>
			if props.State == "partial" {
				<div class="p-4 text-center text-sm text-gray-500">Loading more...</div>
			}
		}
	</div>
}
"@
    "modal.templ" = @"
package components

type ModalProps struct {
	ID    string
	Title string
	State string
}

templ Modal(props ModalProps) {
	<dialog id={ props.ID } class="modal backdrop-blur-sm fixed inset-0 w-full h-full bg-black/50 p-4">
		<div class="modal-box bg-white rounded-lg shadow-xl max-w-lg mx-auto mt-20 p-6 flex flex-col focus-trap">
			<div class="flex justify-between items-center mb-4 border-b pb-2">
				<h3 class="font-bold text-lg">{ props.Title }</h3>
				<form method="dialog">
					<button class="text-gray-500 hover:text-gray-700">✕</button>
				</form>
			</div>
			<div class="modal-body flex-1 overflow-y-auto">
				if props.State == "loading" {
					@SkeletonText(3)
				} else if props.State == "error" {
					<div class="text-red-500">Error loading content</div>
				} else {
					{ children... }
				}
			</div>
		</div>
	</dialog>
}
"@
    "drawer.templ" = @"
package components

type DrawerProps struct {
	ID    string
	Title string
	State string
	Side  string // "left" or "right"
}

templ Drawer(props DrawerProps) {
	<div id={ props.ID } class="drawer fixed inset-0 z-50 flex pointer-events-none">
		<div class="drawer-backdrop fixed inset-0 bg-black/50 pointer-events-auto transition-opacity hidden"></div>
		<div class={ "drawer-panel pointer-events-auto bg-white w-80 h-full shadow-2xl transition-transform transform", templ.KV("translate-x-full ml-auto", props.Side == "right"), templ.KV("-translate-x-full", props.Side == "left") }>
			<div class="p-4 border-b flex justify-between">
				<h2 class="text-lg font-semibold">{ props.Title }</h2>
				<button class="drawer-close">&times;</button>
			</div>
			<div class="p-4 overflow-y-auto h-full">
				if props.State == "loading" {
					@SkeletonText(5)
				} else if props.State == "error" {
					@EmptyState(EmptyStateProps{Title: "Error", Message: "Could not load drawer content"})
				} else {
					{ children... }
				}
			</div>
		</div>
	</div>
}
"@
    "toast.templ" = @"
package components

type ToastProps struct {
	ID      string
	Message string
	Type    string // "success", "error", "info", "warning"
}

templ Toast(props ToastProps) {
	<div id={ props.ID } class="toast fixed bottom-4 right-4 z-50 flex flex-col gap-2" aria-live="polite">
		<div class={ "alert rounded-lg p-4 shadow-lg flex items-center gap-2", 
			templ.KV("bg-green-100 text-green-800", props.Type == "success"),
			templ.KV("bg-red-100 text-red-800", props.Type == "error"),
			templ.KV("bg-blue-100 text-blue-800", props.Type == "info"),
			templ.KV("bg-yellow-100 text-yellow-800", props.Type == "warning") }>
			<span>{ props.Message }</span>
			<button class="ml-auto text-sm opacity-70 hover:opacity-100">&times;</button>
		</div>
	</div>
}
"@
    "tabs.templ" = @"
package components

type Tab struct {
	ID    string
	Label string
}

type TabsProps struct {
	Tabs     []Tab
	ActiveID string
	State    string
}

templ Tabs(props TabsProps) {
	<div class="tabs-container w-full">
		<div class="tab-list flex border-b border-gray-200" role="tablist">
			for _, tab := range props.Tabs {
				<button 
					role="tab"
					aria-selected={ func() string { if tab.ID == props.ActiveID { return "true" } return "false" }() }
					class={ "px-4 py-2 text-sm font-medium border-b-2 transition-colors", 
						templ.KV("border-blue-500 text-blue-600", tab.ID == props.ActiveID),
						templ.KV("border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300", tab.ID != props.ActiveID) }>
					{ tab.Label }
				</button>
			}
		</div>
		<div class="tab-panels p-4">
			if props.State == "loading" {
				@SkeletonText(4)
			} else if props.State == "error" {
				@EmptyState(EmptyStateProps{Title: "Error", Message: "Failed to load tab"})
			} else if props.State == "empty" {
				@EmptyState(EmptyStateProps{Title: "No Content", Message: "This tab is empty"})
			} else {
				{ children... }
			}
		</div>
	</div>
}
"@
    "dropdown.templ" = @"
package components

type DropdownProps struct {
	ID    string
	Label string
	State string
}

templ Dropdown(props DropdownProps) {
	<div class="dropdown relative inline-block text-left" id={ props.ID }>
		<button type="button" class="inline-flex justify-center w-full rounded-md border border-gray-300 shadow-sm px-4 py-2 bg-white text-sm font-medium text-gray-700 hover:bg-gray-50" aria-expanded="false" aria-haspopup="true">
			{ props.Label }
			<svg class="-mr-1 ml-2 h-5 w-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
				<path fill-rule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clip-rule="evenodd" />
			</svg>
		</button>
		<div class="dropdown-menu origin-top-right absolute right-0 mt-2 w-56 rounded-md shadow-lg bg-white ring-1 ring-black ring-opacity-5 hidden focus:outline-none" role="menu" aria-orientation="vertical">
			<div class="py-1" role="none">
				if props.State == "loading" {
					<div class="px-4 py-2 text-sm text-gray-500">Loading...</div>
				} else if props.State == "error" {
					<div class="px-4 py-2 text-sm text-red-500">Error</div>
				} else {
					{ children... }
				}
			</div>
		</div>
	</div>
}
"@
    "datepicker.templ" = @"
package components

type DatePickerProps struct {
	ID       string
	Label    string
	MinDate  string
	MaxDate  string
	State    string
	IsArabic bool
}

templ DatePicker(props DatePickerProps) {
	<div class="datepicker-container flex flex-col gap-1">
		<label for={ props.ID } class="text-sm font-medium text-gray-700">{ props.Label }</label>
		if props.State == "loading" {
			<div class="h-10 w-full bg-gray-200 animate-pulse rounded"></div>
		} else {
			<input 
				type="date" 
				id={ props.ID } 
				name={ props.ID }
				min={ props.MinDate }
				max={ props.MaxDate }
				lang={ func() string { if props.IsArabic { return "ar" } return "en" }() }
				class={ "block w-full rounded-md border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500 sm:text-sm p-2 border",
					templ.KV("border-red-500", props.State == "error") }
			/>
			if props.State == "error" {
				<span class="text-xs text-red-500">Invalid date selection</span>
			}
		}
	</div>
}
"@
    "filedropzone.templ" = @"
package components

import "strconv"

type FileDropzoneProps struct {
	ID       string
	Label    string
	State    string
	Progress int
}

templ FileDropzone(props FileDropzoneProps) {
	<div class="file-dropzone w-full">
		<label class="block text-sm font-medium text-gray-700 mb-1">{ props.Label }</label>
		if props.State == "loading" {
			<div class="h-32 w-full bg-gray-100 animate-pulse rounded-lg border-2 border-dashed border-gray-300"></div>
		} else {
			<div class={ "mt-1 flex justify-center px-6 pt-5 pb-6 border-2 border-dashed rounded-md transition-colors",
				templ.KV("border-gray-300 hover:border-blue-400", props.State != "error"),
				templ.KV("border-red-300 bg-red-50", props.State == "error") }>
				<div class="space-y-1 text-center">
					<svg class="mx-auto h-12 w-12 text-gray-400" stroke="currentColor" fill="none" viewBox="0 0 48 48" aria-hidden="true">
						<path d="M28 8H12a4 4 0 00-4 4v20m32-12v8m0 0v8a4 4 0 01-4 4H12a4 4 0 01-4-4v-4m32-4l-3.172-3.172a4 4 0 00-5.656 0L28 28M8 32l9.172-9.172a4 4 0 015.656 0L28 28m0 0l4 4m4-24h8m-4-4v8m-12 4h.02" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
					</svg>
					<div class="flex text-sm text-gray-600 justify-center">
						<label for={ props.ID } class="relative cursor-pointer bg-white rounded-md font-medium text-blue-600 hover:text-blue-500 focus-within:outline-none focus-within:ring-2 focus-within:ring-offset-2 focus-within:ring-blue-500">
							<span>Upload a file</span>
							<input id={ props.ID } name={ props.ID } type="file" class="sr-only" />
						</label>
						<p class="pl-1">or drag and drop</p>
					</div>
					<p class="text-xs text-gray-500">PNG, JPG, GIF up to 10MB</p>
				</div>
			</div>
			if props.State == "partial" || props.Progress > 0 {
				<div class="w-full bg-gray-200 rounded-full h-2.5 mt-4">
					<div class="bg-blue-600 h-2.5 rounded-full" style={ "width: " + strconv.Itoa(props.Progress) + "%" }></div>
				</div>
			}
			if props.State == "error" {
				<p class="mt-2 text-sm text-red-600">Upload failed. Please try again.</p>
			}
		}
	</div>
}
"@
    "moneydisplay.templ" = @"
package components

import "fmt"

type MoneyDisplayProps struct {
	Amount float64
	State  string
}

templ MoneyDisplay(props MoneyDisplayProps) {
	<span class="money-display inline-flex items-center gap-1">
		if props.State == "loading" {
			<div class="h-6 w-16 bg-gray-200 animate-pulse rounded"></div>
		} else if props.State == "error" {
			<span class="text-red-500 tabular-nums">---</span>
		} else {
			<span class="tabular-nums font-semibold text-gray-900">{ fmt.Sprintf("%.2f", props.Amount) }</span>
			<span class="badge bg-gray-100 text-gray-600 px-1.5 py-0.5 rounded text-xs">EGP</span>
		}
	</span>
}
"@
    "avatar.templ" = @"
package components

type AvatarProps struct {
	ImageURL string
	Initials string
	State    string
}

templ Avatar(props AvatarProps) {
	<div class="avatar relative inline-flex items-center justify-center w-10 h-10 overflow-hidden bg-gray-100 rounded-full">
		if props.State == "loading" {
			<div class="w-full h-full bg-gray-300 animate-pulse"></div>
		} else if props.State == "error" || props.ImageURL == "" {
			<span class="font-medium text-gray-600">{ props.Initials }</span>
		} else {
			<img src={ props.ImageURL } alt="Avatar" class="w-full h-full object-cover" />
		}
	</div>
}
"@
    "skeleton.templ" = @"
package components

templ SkeletonText(lines int) {
	<div class="skeleton-text flex flex-col gap-2 w-full animate-pulse">
		for i := 0; i < lines; i++ {
			<div class={ "h-4 bg-gray-200 rounded", templ.KV("w-3/4", i == lines-1), templ.KV("w-full", i != lines-1) }></div>
		}
	</div>
}

templ SkeletonCard() {
	<div class="skeleton-card border p-4 rounded-lg shadow-sm w-full animate-pulse">
		<div class="h-32 bg-gray-200 rounded-md mb-4"></div>
		<div class="h-6 bg-gray-200 rounded w-1/2 mb-2"></div>
		<div class="h-4 bg-gray-200 rounded w-full mb-1"></div>
		<div class="h-4 bg-gray-200 rounded w-5/6"></div>
	</div>
}

templ SkeletonTable() {
	<div class="skeleton-table w-full animate-pulse border rounded-lg overflow-hidden">
		<div class="h-10 bg-gray-100 border-b"></div>
		for i := 0; i < 5; i++ {
			<div class="flex border-b p-3 gap-4">
				<div class="h-4 bg-gray-200 rounded w-1/4"></div>
				<div class="h-4 bg-gray-200 rounded w-1/4"></div>
				<div class="h-4 bg-gray-200 rounded w-1/4"></div>
				<div class="h-4 bg-gray-200 rounded w-1/4"></div>
			</div>
		}
	</div>
}
"@
    "emptystate.templ" = @"
package components

type EmptyStateProps struct {
	Title       string
	Message     string
	ActionLabel string
}

templ EmptyState(props EmptyStateProps) {
	<div class="empty-state flex flex-col items-center justify-center p-8 text-center border-2 border-dashed border-gray-200 rounded-lg bg-gray-50">
		<div class="illustration mb-4 text-gray-400">
			<svg class="w-16 h-16 mx-auto" fill="none" viewBox="0 0 24 24" stroke="currentColor">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4" />
			</svg>
		</div>
		<h3 class="text-lg font-medium text-gray-900 mb-1">{ props.Title }</h3>
		<p class="text-sm text-gray-500 mb-4 max-w-sm">{ props.Message }</p>
		if props.ActionLabel != "" {
			<button class="px-4 py-2 bg-blue-600 text-white rounded-md text-sm font-medium hover:bg-blue-700 transition-colors">
				{ props.ActionLabel }
			</button>
		}
	</div>
}
"@
}

foreach ($key in $files.Keys) {
    Set-Content -Path "$dir/$key" -Value $files[$key] -Encoding UTF8
}
