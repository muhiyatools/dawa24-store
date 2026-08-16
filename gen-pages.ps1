$dir = "f:\Dawa 24\dawa24-store\internal\ui\pages"
if (!(Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir }

function Write-Templ {
    param($name, $content)
    $content | Out-File -FilePath "$dir\$name" -Encoding utf8
}

Write-Templ "password_reset.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ PasswordReset() {
	@layouts.Base("Reset Password", "en", "ltr") {
		<div class="container">
			<h1>Reset Password</h1>
			<form action="/auth/reset" method="post">
				<input type="email" name="email" placeholder="Email" required />
				<button type="submit" class="btn btn-primary">Send Reset Link</button>
			</form>
		</div>
	}
}

templ PasswordResetConfirm(token string) {
	@layouts.Base("Set New Password", "en", "ltr") {
		<div class="container">
			<h1>Set New Password</h1>
			<form action="/auth/reset/confirm" method="post">
				<input type="hidden" name="token" value={token} />
				<input type="password" name="password" placeholder="New Password" required />
				<button type="submit" class="btn btn-primary">Reset Password</button>
			</form>
		</div>
	}
}
"@

Write-Templ "onboarding.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ Onboarding() {
	@layouts.Base("Onboarding", "en", "ltr") {
		<div class="container">
			<h1>Register Organization</h1>
			<form action="/onboarding" method="post">
				<input type="text" name="orgName" placeholder="Organization Name" required />
				<input type="text" name="taxId" placeholder="Tax ID" required />
				<button type="submit" class="btn btn-primary">Complete Registration</button>
			</form>
		</div>
	}
}
"@

Write-Templ "vendor_products.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ VendorProducts() {
	@layouts.VendorShell("Products", "products", "en", "ltr") {
		<div class="table-container">
			<div class="filters">
				<input type="text" placeholder="Search..." />
				<select><option>All Statuses</option></select>
			</div>
			<table>
				<tr><th>Name</th><th>Status</th></tr>
				<tr><td>Empty</td><td>-</td></tr>
			</table>
			<div class="pagination"></div>
		</div>
	}
}
"@

Write-Templ "vendor_product_editor.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ VendorProductEditor() {
	@layouts.VendorShell("Edit Product", "products", "en", "ltr") {
		<form class="editor-form">
			<div class="grid-2">
				<div>
					<label>English Name</label>
					<input type="text" name="nameEn" />
				</div>
				<div dir="rtl">
					<label>Arabic Name</label>
					<input type="text" name="nameAr" />
				</div>
			</div>
			<div>
				<label>Price</label>
				<input type="number" name="price" />
			</div>
			<div>
				<label>Dosage Form</label>
				<input type="text" name="dosage" />
			</div>
			<div>
				<label>Scientific Name</label>
				<input type="text" name="scientificName" />
			</div>
			<button class="btn btn-primary">Save Product</button>
		</form>
	}
}
"@

Write-Templ "vendor_inventory.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ VendorInventory() {
	@layouts.VendorShell("Inventory", "inventory", "en", "ltr") {
		<div>
			<h2>Stock Levels <span class="badge badge-warning">Low Stock Alerts: 3</span></h2>
			<table>
				<tr><th>Product</th><th>Stock</th><th>Action</th></tr>
				<tr>
					<td>Item A</td>
					<td>
						<input type="number" value="10" />
					</td>
					<td><button class="btn btn-primary">Update</button></td>
				</tr>
			</table>
		</div>
	}
}
"@

Write-Templ "vendor_transfers.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ VendorTransfers() {
	@layouts.VendorShell("Transfers", "inventory", "en", "ltr") {
		<div>
			<button class="btn btn-primary">Initiate Transfer</button>
			<table>
				<tr><th>ID</th><th>From</th><th>To</th><th>Status</th><th>Actions</th></tr>
				<tr>
					<td>#123</td>
					<td>Warehouse A</td>
					<td>Warehouse B</td>
					<td>Pending</td>
					<td>
						<button class="btn btn-primary">Receive</button>
						<button class="btn btn-secondary">Cancel</button>
					</td>
				</tr>
			</table>
		</div>
	}
}
"@

Write-Templ "customer_catalog.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ CustomerCatalog() {
	@layouts.CustomerShell("Catalog", "en", "ltr") {
		<div class="catalog-grid">
			<aside class="filters">
				<h3>Categories</h3>
				<ul><li>Antibiotics</li></ul>
				<h3>Brands</h3>
				<ul><li>PharmaCo</li></ul>
			</aside>
			<main>
				<input type="search" placeholder="Search products..." />
				<div class="grid">
					<div class="card">
						<h4>Product A</h4>
						<p class="tabular-nums">150.00 EGP</p>
						<button class="btn btn-primary">Add to Cart</button>
					</div>
				</div>
			</main>
		</div>
	}
}
"@

Write-Templ "customer_product_detail.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ CustomerProductDetail() {
	@layouts.CustomerShell("Product Detail", "en", "ltr") {
		<div class="product-detail">
			<h1>Product Name</h1>
			<p>Dosage Form: Tablet</p>
			<p>Manufacturer: PharmaCo</p>
			<p class="tabular-nums">150.00 EGP</p>
			<button class="btn btn-primary">Add to Cart</button>
		</div>
	}
}
"@

Write-Templ "customer_cart.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ CustomerCart() {
	@layouts.CustomerShell("Shopping Cart", "en", "ltr") {
		<div class="cart">
			<h2>Your Cart</h2>
			<ul>
				<li>
					Product A - 
					<input type="number" value="1" />
					<span class="tabular-nums">150.00 EGP</span>
				</li>
			</ul>
			<div class="subtotal tabular-nums">Subtotal: 150.00 EGP</div>
			<a href="/checkout" class="btn btn-primary">Proceed to Checkout</a>
		</div>
	}
}
"@

Write-Templ "customer_checkout.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ CustomerCheckout() {
	@layouts.CustomerShell("Checkout", "en", "ltr") {
		<div class="checkout">
			<h2>Checkout</h2>
			<div class="summary">
				<h3>Order Summary</h3>
				<p>Vendor 1: 150.00 EGP</p>
			</div>
			<form action="/checkout" method="post">
				<select name="address">
					<option>Main Pharmacy - 123 Street</option>
				</select>
				<select name="payment">
					<option>Cash on Delivery</option>
					<option>Bank Transfer</option>
				</select>
				<button type="submit" class="btn btn-primary">Place Order</button>
			</form>
		</div>
	}
}
"@

Write-Templ "customer_orders.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ CustomerOrders() {
	@layouts.CustomerShell("My Orders", "en", "ltr") {
		<div class="orders">
			<h2>Order History</h2>
			<ul>
				<li>
					<a href="/orders/1">Order #1</a> - Processing
				</li>
			</ul>
			<div class="timeline">
				<p>Order Tracking Timeline:</p>
				<ul>
					<li>Placed</li>
					<li>Processing</li>
					<li>Shipped</li>
				</ul>
			</div>
		</div>
	}
}
"@

Write-Templ "vendor_orders.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ VendorOrders() {
	@layouts.VendorShell("Orders", "shipments", "en", "ltr") {
		<div class="orders">
			<h2>Fulfilment</h2>
			<table>
				<tr><th>Order ID</th><th>Status</th><th>Actions</th></tr>
				<tr>
					<td>#1001</td>
					<td>Pending</td>
					<td>
						<button class="btn btn-primary">Process</button>
						<button class="btn btn-secondary">Ship</button>
					</td>
				</tr>
			</table>
		</div>
	}
}
"@

Write-Templ "vendor_offers.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ VendorOffers() {
	@layouts.VendorShell("Offers", "offers", "en", "ltr") {
		<div class="offers">
			<h2>Promotions & Ad Campaigns</h2>
			<button class="btn btn-primary">Create Promotion</button>
			<ul>
				<li>Summer Sale - 10% Off</li>
			</ul>
		</div>
	}
}
"@

Write-Templ "admin_users.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ AdminUsers() {
	@layouts.AdminShell("Users", "users", "en", "ltr") {
		<div class="users">
			<select><option>All Roles</option></select>
			<table>
				<tr><th>User</th><th>Role</th><th>Actions</th></tr>
				<tr>
					<td>admin@dawa24.com</td>
					<td>Admin</td>
					<td>
						<button class="btn btn-secondary">Suspend</button>
						<button class="btn btn-secondary">Reset MFA</button>
					</td>
				</tr>
			</table>
		</div>
	}
}
"@

Write-Templ "admin_settings.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ AdminSettings() {
	@layouts.AdminShell("Settings", "system", "en", "ltr") {
		<div class="settings">
			<h2>Reference Data</h2>
			<form>
				<label>Currencies</label>
				<input type="text" value="EGP, USD" />
				<label>Languages</label>
				<input type="text" value="en, ar" />
				<label>Contact Email</label>
				<input type="email" value="support@dawa24.com" />
				<button class="btn btn-primary">Save Settings</button>
			</form>
		</div>
	}
}
"@

Write-Templ "notifications.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ Notifications() {
	@layouts.Base("Notifications", "en", "ltr") {
		<div class="notifications">
			<h2>Notifications <span class="badge">3 Unread</span></h2>
			<button class="btn btn-secondary">Mark All Read</button>
			<ul>
				<li class="unread">New Order Received</li>
				<li>Password Changed</li>
			</ul>
		</div>
	}
}
"@

Write-Templ "public_pages.templ" @"
package pages
import "github.com/muhiya/dawa24-store/internal/ui/layouts"

templ PrivacyPolicy() {
	@layouts.Base("Privacy Policy", "ar", "rtl") {
		<div class="content" dir="rtl">
			<h1>سياسة الخصوصية</h1>
			<p>نحن نقدر خصوصيتك...</p>
		</div>
	}
}

templ TermsOfService() {
	@layouts.Base("Terms of Service", "ar", "rtl") {
		<div class="content" dir="rtl">
			<h1>شروط الخدمة</h1>
			<p>باستخدامك لموقعنا...</p>
		</div>
	}
}
"@
