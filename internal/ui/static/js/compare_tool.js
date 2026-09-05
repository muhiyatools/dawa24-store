function toggleSelectAll(masterCheckbox) {
	const checkboxes = document.querySelectorAll('.file-checkbox');
	checkboxes.forEach(cb => cb.checked = masterCheckbox.checked);
}

function validateCompareSelection(event) {
	const checked = document.querySelectorAll('.file-checkbox:checked');
	if (checked.length === 0) {
		const all = document.querySelectorAll('.file-checkbox');
		if (all.length > 0) {
			all.forEach(cb => cb.checked = true);
			return true;
		}
		alert("يرجى رفع ملف كشف أسعار واحد على الأقل للمقارنة.");
		event.preventDefault();
		return false;
	}
	return true;
}

function triggerFileInput() {
	const input = document.getElementById('file-upload-input');
	if (input) {
		input.click();
	}
}

function handleFileSelect(input) {
	const previewEl = document.getElementById('file-name-preview');
	if (!previewEl) return;

	if (input.files && input.files.length > 0) {
		if (input.files.length === 1) {
			const file = input.files[0];
			const size = (file.size / 1024 / 1024).toFixed(2);
			previewEl.innerHTML = '<span>تم اختيار ملف:</span> <strong class=\"text-primary\">' + file.name + '</strong> (' + size + ' MB)';
		} else {
			let fileNames = [];
			let totalSize = 0;
			for (let i = 0; i < input.files.length; i++) {
				fileNames.push(input.files[i].name);
				totalSize += input.files[i].size;
			}
			const totalMb = (totalSize / 1024 / 1024).toFixed(2);
			previewEl.innerHTML = '<div class=\"stack-sm\"><div class=\"stat-card-label\">تم اختيار (' + input.files.length + ') ملفات موردين (الإجمالي: ' + totalMb + ' MB):</div><div class=\"stack-sm\">' + fileNames.map(n => '• ' + n).join('<br/>') + '</div></div>';
		}
	} else {
		previewEl.innerHTML = '';
	}
}

(function setupDropZone() {
	window.addEventListener('DOMContentLoaded', () => {
		const dropZone = document.getElementById('drop-zone-box');
		const fileInput = document.getElementById('file-upload-input');
		if (!dropZone || !fileInput) return;

		['dragenter', 'dragover'].forEach(eventName => {
			dropZone.addEventListener(eventName, (e) => {
				e.preventDefault();
				e.stopPropagation();
				dropZone.style.borderColor = 'var(--accent)';
				dropZone.style.background = 'var(--surface-raised)';
			}, false);
		});

		['dragleave', 'drop'].forEach(eventName => {
			dropZone.addEventListener(eventName, (e) => {
				e.preventDefault();
				e.stopPropagation();
				dropZone.style.borderColor = '';
				dropZone.style.background = '';
			}, false);
		});

		dropZone.addEventListener('drop', (e) => {
			const dt = e.dataTransfer;
			const files = dt.files;
			if (files && files.length > 0) {
				fileInput.files = files;
				handleFileSelect(fileInput);
			}
		}, false);
	});
})();

/*
 * The batch upload, with the two things it never had: a measured progress bar,
 * and a limit that takes what fits instead of refusing everything.
 *
 * It used to grey the button out and submit the form normally. A normal submit
 * gives the browser no way to report how far the transfer has got, so ten price
 * lists went up over a minute and a half with the page showing nothing - which
 * looks exactly like a dead connection, and people reloaded, which abandoned
 * the upload and started it over.
 */
function handleUploadSubmit(event) {
	const form = document.getElementById('compare-upload-form');
	const fileInput = document.getElementById('file-upload-input');
	const dropZone = document.getElementById('drop-zone-box');

	if (!fileInput || !fileInput.files || fileInput.files.length === 0) {
		alert("يرجى اختيار ملف Excel أو CSV واحد على الأقل للرفع والمعالجة.");
		event.preventDefault();
		triggerFileInput();
		return false;
	}

	event.preventDefault();

	let selected = Array.from(fileInput.files);
	let skipped = [];

	// Over the plan limit: take the ones that fit and say which did not.
	//
	// This used to be an alert that refused the whole batch and left the files
	// on the user's desk to sort out by hand. The server does the same trimming
	// authoritatively - this is here so the dialog can NAME the files that will
	// not be taken before anything is sent, rather than reporting it afterwards.
	if (dropZone) {
		const currentCount = parseInt(dropZone.dataset.currentCount || '0', 10);
		const maxLimit = parseInt(dropZone.dataset.maxLimit || '0', 10);
		const room = maxLimit > 0 ? maxLimit - currentCount : selected.length;
		if (maxLimit > 0 && room <= 0) {
			alert('باقتك الحالية تسمح بحد أقصى ' + maxLimit + ' كشوف، وجميعها مستخدمة. احذف أو أرشف كشفاً قديماً، أو رقِّ الباقة، ثم أعد المحاولة.');
			return false;
		}
		if (maxLimit > 0 && selected.length > room) {
			skipped = selected.slice(room);
			selected = selected.slice(0, room);
		}
	}

	// Hand the trimmed selection back to the input so FormData sends exactly
	// what the dialog says it is sending.
	if (skipped.length && typeof DataTransfer === 'function') {
		const keep = new DataTransfer();
		selected.forEach((f) => keep.items.add(f));
		fileInput.files = keep.files;
	}

	const btn = document.getElementById('upload-submit-btn');
	const btnText = document.getElementById('upload-btn-text');
	if (btn && btnText) {
		btn.disabled = true;
		btn.style.opacity = '0.75';
		btnText.textContent = 'جارٍ رفع (' + selected.length + ') كشوف…';
	}

	// No dialog available (an older browser, or the component was not rendered):
	// fall back to the ordinary submit rather than doing nothing at all.
	if (typeof window.UploadProgress !== 'function' || !document.getElementById('compare-upload-progress')) {
		form.submit();
		return false;
	}

	const bar = new window.UploadProgress('compare-upload-progress');
	const listed = selected.map((f) => ({ name: f.name }))
		.concat(skipped.map((f) => ({ name: f.name, state: 'skipped', label: 'يتجاوز حد الباقة' })));

	bar.submit(form, {
		files: listed,
		uploadingCaption: 'جارٍ رفع (' + selected.length + ') من كشوف الموردين…',
		processingCaption: 'تم الرفع. جارٍ قراءة الكشوف ومطابقة الأصناف بالكتالوج…',
	});

	if (skipped.length) {
		bar.setDetail(skipped.length + ' ملف لم يُرفع لتجاوز حد الباقة');
	}
	return false;
}

function openRenameModal(fileId, currentName) {
	const modal = document.getElementById('rename-file-modal');
	const form = document.getElementById('rename-file-form');
	const input = document.getElementById('rename-supplier-input');
	if (modal && form && input) {
		form.action = '/compare/files/' + fileId + '/rename';
		input.value = currentName || '';
		modal.classList.remove('d-none');
		modal.classList.add('d-flex');
		setTimeout(() => input.focus(), 50);
	}
}

function closeRenameModal() {
	const modal = document.getElementById('rename-file-modal');
	if (modal) {
		modal.classList.add('d-none');
		modal.classList.remove('d-flex');
	}
}

function deleteFileConfirm(id) {
	if (confirm("هل أنت متأكد من حذف هذا الملف نهائياً من مركز الملفات؟")) {
		const form = document.createElement('form');
		form.method = 'POST';
		form.action = '/compare/files/' + id + '/delete';
		document.body.appendChild(form);
		form.submit();
	}
}

let searchTimeout = null;
function filterSearchLocal(query) {
	clearTimeout(searchTimeout);
	const resultsContainer = document.getElementById('instant-search-results');
	if (!query || query.trim().length < 2) {
		resultsContainer.innerHTML = '<div class=\"stack-sm\">اكتب اسم الصنف للبحث المباشر عبر جميع كشوف الموردين المرفوعة ومقارنة الخصومات فورياً.</div>';
		return;
	}

	resultsContainer.innerHTML = '<div class=\"stack-sm\">⏳ جاري البحث عبر الكتالوج وكشوف الموردين...</div>';

	searchTimeout = setTimeout(() => {
		fetch('/compare/search?q=' + encodeURIComponent(query.trim()), {
			headers: { 'Accept': 'application/json' }
		})
		.then(r => {
			if (!r.ok) throw new Error('Search failed');
			return r.json();
		})
		.then(data => {
			const items = data.items || [];
			if (items.length === 0) {
				resultsContainer.innerHTML = '<div class=\"stack-sm\">لم يتم العثور على أصناف مطابقة لـ \"' + query + '\".</div>';
				return;
			}

			let html = '<div class=\"stack-sm\"><span>تم العثور على <strong>' + items.length + '</strong> صنف</span><span>بالكتالوج: ' + (data.in_catalog_count || 0) + ' | أصناف موردين: ' + (data.custom_items_count || 0) + '</span></div>';

			items.slice(0, 15).forEach(item => {
				let statusBadge = '';
				if (item.catalog_status === 'catalog_and_suppliers') {
					statusBadge = '<span class=\"badge badge-emerald\">معتمد بالكتالوج ومتوفر</span>';
				} else if (item.catalog_status === 'catalog_only') {
					statusBadge = '<span class=\"badge badge-sky\">مسجل بالكتالوج (غير متوفر بالكشوف)</span>';
				} else {
					statusBadge = '<span class=\"badge badge-amber\">صنف خاص بكشف المورد</span>';
				}

				let bestPriceHtml = '';
				if (item.best_net_price && parseFloat(item.best_net_price) > 0) {
					bestPriceHtml = '<div class=\"text-end\"><div class=\"tabular-nums\">' + item.best_net_price + ' ج.م</div><div class=\"stack-sm\">' + (item.best_supplier || 'أفضل سعر') + ' (' + (item.best_discount || 0) + '%)</div></div>';
				} else {
					bestPriceHtml = '<div class=\"stack-sm\">لا توجد عروض أسعار مرفوعة</div>';
				}

				let offersHtml = '';
				if (item.offers && Object.keys(item.offers).length > 0) {
					offersHtml = '<div class=\"offers-list d-flex flex-wrap gap-2 mt-2\">';
					for (const [sup, off] of Object.entries(item.offers)) {
						const isBest = (sup === item.best_supplier);
						const badgeClass = isBest ? 'badge-emerald' : 'badge-secondary';
						offersHtml += '<span class=\"badge ' + badgeClass + '\">' + sup + ': ' + (off.discount || 0) + '% (' + (off.price_after_discount || '--') + ' ج.م)</span>';
					}
					offersHtml += '</div>';
				}

				const skuHtml = item.sku ? ('<span class=\"tabular-nums text-xs text-secondary\">كود: ' + item.sku + '</span>') : '';
				html += '<div class=\"comparison-card card p-4 mb-3\"><div class=\"d-flex justify-between items-start flex-wrap gap-3\"><div class=\"stack-xs\"><div class=\"d-flex items-center gap-2\"><strong class=\"product-title font-bold text-base\">' + (item.product_name || 'صنف دوائي') + '</strong>' + statusBadge + '</div>' + skuHtml + '</div>' + bestPriceHtml + '</div>' + offersHtml + '</div>';
			});

			resultsContainer.innerHTML = html;
		})
		.catch(err => {
			resultsContainer.innerHTML = '<div class=\"stat-card-label\">حدث خطأ أثناء البحث. يرجى المحاولة بكلمة بحث أخرى.</div>';
		});
	}, 250);
}

let isModalLoading = false;

function openSetupModal(fileId, queue, step, total) {
	if (isModalLoading) return;
	const root = document.getElementById('mapping-modal-root');
	if (!root) return;

	isModalLoading = true;
	step = step || 1;
	total = total || 1;
	queue = queue || '';

	const existingDialog = root.querySelector('dialog');
	const existingBox = existingDialog ? existingDialog.querySelector('.modal-box') : null;

	if (existingBox) {
		const submitBtn = existingBox.querySelector('#mapping-submit-btn');
		if (submitBtn) {
			submitBtn.disabled = true;
			submitBtn.innerHTML = '⏳ جاري الانتقال للملف التالي...';
		}
		const form = existingBox.querySelector('#compare-mapping-form');
		if (form) {
			form.classList.add('opacity-50', 'pointer-events-none');
		}
	} else {
		root.innerHTML = '<dialog id=\"compare-mapping-modal-backdrop\" class=\"modal\" open><div class=\"modal-box modal-xl\"><div class=\"modal-header\"><div class=\"d-flex items-center gap-2\"><span class=\"text-primary font-bold\">⚙️</span><div><h3 class=\"modal-title\">معالج ضبط أعمدة الملفات (' + step + ' من ' + total + ')</h3><p class=\"text-xs text-muted mt-0.5 m-0\">جاري قراءة بيانات الكشف وإعداد المعاينة الذكية...</p></div></div><form class=\"m-0\" method=\"dialog\"><button type=\"button\" class=\"modal-close\" onclick=\"closeMappingModal()\" aria-label=\"إغلاق\">✕</button></form></div><div class=\"modal-body p-6 text-center\"><div class=\"py-8 text-muted\"><div class=\"text-2xl mb-2\">⏳</div><div class=\"font-bold text-sm\">جاري قراءة أعمدة الملف وتحليل المحتوى...</div></div></div></div></dialog>';
	}

	fetch('/compare/files/' + fileId + '/mapping-modal?setup=1&queue=' + encodeURIComponent(queue) + '&step=' + step + '&total=' + total)
		.then(res => {
			if (!res.ok) throw new Error('فشل فتح معالج ضبط الأعمدة');
			return res.text();
		})
		.then(html => {
			isModalLoading = false;
			const parser = new DOMParser();
			const doc = parser.parseFromString(html, 'text/html');
			const newDialog = doc.querySelector('dialog');
			const currentDialog = root.querySelector('dialog');

			if (currentDialog && newDialog) {
				const newBox = newDialog.querySelector('.modal-box');
				const currentBox = currentDialog.querySelector('.modal-box');
				if (newBox && currentBox) {
					currentBox.replaceWith(newBox);
					return;
				}
			}

			root.innerHTML = html;
		})
		.catch(err => {
			isModalLoading = false;
			alert('تعذر فتح معالج ضبط الأعمدة: ' + err.message);
			closeMappingModal();
		});
}

function openMappingModal(fileId) {
	if (isModalLoading) return;
	const root = document.getElementById('mapping-modal-root');
	if (!root) return;

	isModalLoading = true;
	root.innerHTML = '<dialog id=\"compare-mapping-modal-backdrop\" class=\"modal\" open><div class=\"modal-box modal-xl\"><div class=\"modal-header\"><div class=\"d-flex items-center gap-2\"><span class=\"text-primary font-bold\">⚙️</span><h3 class=\"modal-title\">تعيين أعمدة كشف المورد</h3></div><form class=\"m-0\" method=\"dialog\"><button type=\"button\" class=\"modal-close\" onclick=\"closeMappingModal()\" aria-label=\"إغلاق\">✕</button></form></div><div class=\"modal-body p-6 text-center\"><div class=\"py-8 text-muted\"><div class=\"text-2xl mb-2\">⏳</div><div class=\"font-bold text-sm\">جاري قراءة أعمدة الملف ومعاينتها...</div></div></div></div></dialog>';

	fetch('/compare/files/' + fileId + '/mapping-modal')
		.then(res => {
			if (!res.ok) throw new Error('فشل جلب نافذة تعيين الأعمدة');
			return res.text();
		})
		.then(html => {
			isModalLoading = false;
			root.innerHTML = html;
		})
		.catch(err => {
			isModalLoading = false;
			alert('تعذر فتح نافذة تعيين الأعمدة: ' + err.message);
			closeMappingModal();
		});
}

function submitMappingFormAsync(event) {
	event.preventDefault();
	const form = event.target;
	const submitBtn = document.getElementById('mapping-submit-btn');
	if (submitBtn) {
		submitBtn.disabled = true;
		submitBtn.style.opacity = '0.7';
		submitBtn.innerHTML = '⏳ جاري الحفظ والمعالجة...';
	}

	const formData = new FormData(form);

	fetch(form.action, {
		method: 'POST',
		body: formData,
		headers: {
			'Accept': 'application/json'
		}
	})
	.then(res => {
		if (!res.ok) throw new Error('حدث خطأ أثناء حفظ تعيين الأعمدة.');
		return res.json();
	})
	.then(data => {
		if (data.next_file_id && data.next_file_id > 0) {
			openSetupModal(data.next_file_id, data.remaining_queue, data.step, data.total);
		} else {
			closeMappingModal();
			window.location.href = '/compare/tool?notice=success&msg=' + encodeURIComponent('تم حفظ وتطبيق ضبط أعمدة كافة ملفات الموردين بنجاح.');
		}
	})
	.catch(err => {
		alert(err.message || 'تعذر حفظ تعيين الأعمدة.');
		if (submitBtn) {
			submitBtn.disabled = false;
			submitBtn.style.opacity = '1';
			submitBtn.innerHTML = 'حفظ وإعادة المحاولة';
		}
	});

	return false;
}

function handleSetupSkip(fileId, remainingQueue, step, total) {
	if (!confirm('هل أنت متأكد من تخطي هذا الملف؟ سيتم حذفه من المقارنة والانتقال للملف التالي.')) {
		return;
	}

	const root = document.getElementById('mapping-modal-root');
	const currentDialog = root ? root.querySelector('dialog') : null;
	if (currentDialog) {
		const form = currentDialog.querySelector('#compare-mapping-form');
		if (form) {
			form.classList.add('opacity-50', 'pointer-events-none');
		}
		const submitBtn = currentDialog.querySelector('#mapping-submit-btn');
		if (submitBtn) {
			submitBtn.disabled = true;
			submitBtn.innerHTML = '⏩ جاري تخطي الملف...';
		}
	}

	const formData = new FormData();
	formData.append('setup_queue', remainingQueue);
	formData.append('step', step);
	formData.append('total', total);

	fetch('/compare/files/' + fileId + '/skip', {
		method: 'POST',
		body: formData,
		headers: {
			'Accept': 'application/json'
		}
	})
	.then(res => {
		if (!res.ok) throw new Error('فشل تخطي الملف.');
		return res.json();
	})
	.then(data => {
		if (data.next_file_id && data.next_file_id > 0) {
			openSetupModal(data.next_file_id, data.remaining_queue, data.step, data.total);
		} else {
			closeMappingModal();
			window.location.href = '/compare/tool?notice=success&msg=' + encodeURIComponent('تم تخطي الملف والانتهاء من معالج الإعداد.');
		}
	})
	.catch(err => {
		alert(err.message || 'تعذر تخطي الملف.');
		closeMappingModal();
	});
}

function closeMappingModal() {
	const root = document.getElementById('mapping-modal-root');
	if (root) {
		const dialog = root.querySelector('dialog');
		if (dialog && typeof dialog.close === 'function') {
			try { dialog.close(); } catch(e) {}
		}
		root.innerHTML = '';
	}
}

window.addEventListener('DOMContentLoaded', () => {
	const params = new URLSearchParams(window.location.search);
	const setupFile = params.get('setup_file');
	const setupQueue = params.get('setup_queue');
	const setupStep = parseInt(params.get('setup_step') || '1', 10);
	const setupTotal = parseInt(params.get('setup_total') || '1', 10);

	if (setupFile) {
		openSetupModal(setupFile, setupQueue, setupStep, setupTotal);
	} else {
		const openFileId = params.get('open_mapping') || params.get('mapping_file');
		if (openFileId) {
			openMappingModal(openFileId);
		}
	}
});