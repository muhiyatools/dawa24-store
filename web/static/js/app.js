// Dawa24 Store UI Client
const state = {
  activeTab: 'catalog',
  cart: [],
  products: [
    { id: 1, name: 'كونكور 5 مجم (Concor 5mg)', category: 'أدوية القلب والضغط', pharma: 'بيسوبرولول - 30 قرص', price: '45.00', originalPrice: '52.00', vendor: 'مخزن المتحدة للأدوية' },
    { id: 2, name: 'أوجمنتين 1 جم (Augmentin 1g)', category: 'مضادات حيوية', pharma: 'أموكسيسيلين + كلافولانات', price: '90.00', originalPrice: '99.50', vendor: 'الشركة المصرية لتجارة الأدوية' },
    { id: 3, name: 'بانادول إكسترا (Panadol Extra)', category: 'مسكنات وخافضات حرارة', pharma: 'باراسيتامول + كافيين', price: '32.00', originalPrice: '38.00', vendor: 'مخزن النور فارما' },
    { id: 4, name: 'كتافلام 50 مجم (Cataflam 50mg)', category: 'مسكن ومضاد للالتهاب', pharma: 'ديكلوفيناك بوتاسيوم', price: '35.50', originalPrice: '41.00', vendor: 'مخزن الدلتا للأدوية' },
    { id: 5, name: 'أنتينال كبسول (Antinal Caps)', category: 'مطهرات معوية', pharma: 'نيفوروكسازيد 200 مجم', price: '26.00', originalPrice: '30.00', vendor: 'مخزن المتحدة للأدوية' },
    { id: 6, name: 'سيبتازول أقراص (Septazole Tabs)', category: 'مضاد للبكتيريا', pharma: 'سلفاميثوكسازول + تريميثوبريم', price: '18.00', originalPrice: '21.00', vendor: 'الشركة المصرية لتجارة الأدوية' }
  ],
  offers: [
    { id: 101, title: 'خصم إضافي 15% على مستلزمات العناية', vendor: 'مخزن الأمل', discount: '15%', minOrder: '500 ج.م' },
    { id: 102, title: 'شحن مجاني للطلبات فوق 2000 ج.م', vendor: 'مخزن المتحدة', discount: 'شحن مجاني', minOrder: '2000 ج.م' }
  ],
  orders: []
};

function initApp() {
  renderProducts(state.products);
  setupListeners();
}

function renderProducts(items) {
  const container = document.getElementById('products-grid');
  if (!container) return;

  if (items.length === 0) {
    container.innerHTML = '<div style="grid-column: 1/-1; text-align: center; padding: 3rem; color: var(--text-muted);">لا توجد منتجات مطابقة لعملية البحث</div>';
    return;
  }

  container.innerHTML = items.map(p => `
    <div class="product-card">
      <div>
        <span class="product-tag">${p.category}</span>
        <h3 class="product-name">${p.name}</h3>
        <p class="product-pharma">${p.pharma}</p>
        <p style="font-size: 0.8rem; color: var(--text-muted); margin-bottom: 0.5rem;">المورد: ${p.vendor}</p>
      </div>
      <div>
        <div class="product-pricing">
          <span class="price-current">${p.price} ج.م</span>
          <span class="price-original">${p.originalPrice} ج.م</span>
        </div>
        <button class="btn-add-cart" onclick="addToCart(${p.id})">
          <span>إضافة إلى الطلب</span>
          <span>+</span>
        </button>
      </div>
    </div>
  `).join('');
}

function setupListeners() {
  const searchInput = document.getElementById('search-input');
  if (searchInput) {
    searchInput.addEventListener('input', (e) => {
      const q = e.target.value.trim().toLowerCase();
      if (!q) {
        renderProducts(state.products);
        return;
      }
      const filtered = state.products.filter(p => 
        p.name.toLowerCase().includes(q) || 
        p.pharma.toLowerCase().includes(q) ||
        p.category.toLowerCase().includes(q)
      );
      renderProducts(filtered);
    });
  }
}

function addToCart(productId) {
  const product = state.products.find(p => p.id === productId);
  if (!product) return;

  const existing = state.cart.find(item => item.product.id === productId);
  if (existing) {
    existing.quantity++;
  } else {
    state.cart.push({ product, quantity: 1 });
  }

  updateCartCounter();
  showToast(`تمت إضافة "${product.name}" إلى السلة`);
}

function updateCartCounter() {
  const counter = document.getElementById('cart-counter');
  if (counter) {
    const total = state.cart.reduce((sum, item) => sum + item.quantity, 0);
    counter.textContent = total;
  }
}

function toggleCartDrawer(open) {
  const overlay = document.getElementById('cart-drawer');
  if (!overlay) return;
  if (open) {
    renderCartItems();
    overlay.classList.add('active');
  } else {
    overlay.classList.remove('active');
  }
}

function renderCartItems() {
  const list = document.getElementById('cart-items-list');
  const totalElem = document.getElementById('cart-total');
  if (!list) return;

  if (state.cart.length === 0) {
    list.innerHTML = '<div style="text-align: center; padding: 2rem; color: var(--text-muted);">سلة الطلبات فارغة</div>';
    if (totalElem) totalElem.textContent = '0.00 ج.م';
    return;
  }

  let grandTotal = 0;
  list.innerHTML = state.cart.map((item, idx) => {
    const subtotal = parseFloat(item.product.price) * item.quantity;
    grandTotal += subtotal;
    return `
      <div class="cart-item-row">
        <div>
          <div style="font-weight: 700; font-size: 0.95rem;">${item.product.name}</div>
          <div style="font-size: 0.8rem; color: var(--text-muted);">${item.product.price} ج.م × ${item.quantity}</div>
        </div>
        <div style="text-align: left;">
          <div style="font-weight: 800; color: var(--primary);">${subtotal.toFixed(2)} ج.م</div>
          <button style="background: none; border: none; color: var(--danger); font-size: 0.75rem; cursor: pointer;" onclick="removeFromCart(${idx})">حذف</button>
        </div>
      </div>
    `;
  }).join('');

  if (totalElem) totalElem.textContent = `${grandTotal.toFixed(2)} ج.م`;
}

function removeFromCart(idx) {
  state.cart.splice(idx, 1);
  updateCartCounter();
  renderCartItems();
}

function executeCheckout() {
  if (state.cart.length === 0) return;
  const orderNum = 'ORD-' + Math.floor(100000 + Math.random() * 900000);
  const total = state.cart.reduce((sum, item) => sum + (parseFloat(item.product.price) * item.quantity), 0);
  
  state.orders.unshift({
    number: orderNum,
    date: new Date().toLocaleDateString('ar-EG'),
    total: total.toFixed(2),
    itemsCount: state.cart.length,
    status: 'مكتمل التأكيد'
  });

  state.cart = [];
  updateCartCounter();
  toggleCartDrawer(false);
  showToast(`تم إنشاء الطلب بنجاح برقم: ${orderNum}`);
}

function showToast(msg) {
  const container = document.getElementById('toast-container');
  if (!container) return;
  const toast = document.createElement('div');
  toast.className = 'toast';
  toast.textContent = msg;
  container.appendChild(toast);
  setTimeout(() => toast.remove(), 3000);
}

document.addEventListener('DOMContentLoaded', initApp);
