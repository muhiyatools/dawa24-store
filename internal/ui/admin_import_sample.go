package ui

// The downloadable import template.
//
// One definition backs both the .xlsx and the .csv download, because two copies
// drift: the CSV template shipped ten columns and the Excel template shipped a
// different ten, and neither carried a SKU column — so an admin who filled in
// the supplied template had no identifier for the importer to match on, and
// every re-upload of a corrected file created a second copy of every product.
//
// A round-trip test parses this exact content back through the importer, so the
// file the admin downloads is guaranteed to be one the importer reads correctly.

// importSampleHeaders are written in the vocabulary the column mapper scores
// highest, so a template filled in as-is maps with full confidence.
var importSampleHeaders = []string{
	"اسم الصنف بالعربي",
	"اسم الصنف بالإنجليزي",
	"كود الصنف",
	"الباركود",
	"الاسم العلمي",
	"المادة الفعالة",
	"الشكل الصيدلي",
	"التركيز",
	"الوحدة",
	"الشركة المصنعة",
	"سعر البيع",
	"سعر الجمهور",
	"نسبة الخصم",
	"الوصف بالعربي",
	"الوصف بالإنجليزي",
}

// importSampleRows show one filled example per common dosage form, so the shape
// of each column is unambiguous — particularly the price format and the
// percentage discount, the two that supplier files most often get wrong.
var importSampleRows = [][]string{
	{
		"كونجستال أقراص", "Congestal Tablets", "CONG-TAB-650", "6221234567890",
		"Paracetamol + Pseudoephedrine", "Paracetamol 500mg", "أقراص", "650mg", "علبة",
		"Eva Pharma", "25.00", "30.00", "10%",
		"لعلاج أعراض نزلات البرد والإنفلونزا", "For cold and flu relief",
	},
	{
		"بانادول إكسترا", "Panadol Extra", "PAN-EXT-500", "6229876543210",
		"Paracetamol + Caffeine", "Paracetamol 500mg + Caffeine 65mg", "أقراص", "500mg", "علبة",
		"GSK", "35.00", "40.00", "5%",
		"مسكن للآلام وخافض للحرارة", "Pain reliever and fever reducer",
	},
	{
		"أوجمنتين 1 جم أقراص", "Augmentin 1g Tablets", "AUG-1G", "6223334445556",
		"Amoxicillin + Clavulanic Acid", "Amoxicillin 875mg + Clavulanate 125mg", "أقراص", "1g", "علبة",
		"GlaxoSmithKline", "89.50", "95.00", "",
		"مضاد حيوي واسع المجال", "Broad spectrum antibiotic",
	},
	{
		"أنتينال كبسول", "Antinal Capsules", "ANTIN-CAP-200", "6224445556667",
		"Nifuroxazide", "Nifuroxazide 200mg", "كبسولات", "200mg", "شريط",
		"Amoun Pharmaceutical", "30.00", "", "",
		"مطهر معوي ومضاد للإسهال", "Intestinal antiseptic",
	},
	{
		"كتافاست فوار", "Catafast Sachets", "CATA-SACH-50", "6227778889990",
		"Diclofenac Potassium", "Diclofenac Potassium 50mg", "أكياس فوار", "50mg", "ظرف",
		"Novartis", "65.00", "70.00", "7%",
		"مسكن سريع المفعول ومضاد للالتهاب", "Fast acting pain relief",
	},
}
