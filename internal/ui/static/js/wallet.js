/*
 * The wallet page's payment-method dialog.
 *
 * One dialog serves both adding and editing. There was no edit dialog before:
 * POST /settings/payment-methods/{id}/edit existed and no screen could open it,
 * because only the rendered line — "CIB • أحمد • IBAN: EG38..." — was stored,
 * and there is no way back from that sentence to the three inputs that made it.
 * The fields are kept alongside it now, and each row carries them in a data
 * attribute for this to read.
 */
(function () {
	'use strict';

	var EMPTY = {
		account_holder: '',
		bank_name: '',
		iban: '',
		account_number: '',
		instapay_handle: '',
		wallet_provider: '',
		wallet_phone: '',
		card_brand: '',
		card_last4: '',
		is_default: false
	};

	function dawaWallet(config) {
		return {
			activeSection: 'transactions',
			isDepositModalOpen: false,
			isWithdrawModalOpen: false,
			isAddPaymentModalOpen: false,

			base: (config && config.base) || '/customer/wallet',
			paymentType: 'bank',
			paymentEditID: 0,
			paymentForm: Object.assign({}, EMPTY),

			openPaymentAdd() {
				this.paymentEditID = 0;
				this.paymentType = 'bank';
				this.paymentForm = Object.assign({}, EMPTY);
				this.isAddPaymentModalOpen = true;
			},

			openPaymentEdit(el) {
				var pm;
				try { pm = JSON.parse(el.dataset.paymentMethod || '{}'); } catch (e) { return; }

				this.paymentEditID = pm.id || 0;
				this.paymentType = pm.type || 'bank';
				this.paymentForm = Object.assign({}, EMPTY, {
					account_holder: pm.account_holder || '',
					bank_name: pm.bank_name || '',
					iban: pm.iban || '',
					account_number: pm.account_number || '',
					instapay_handle: pm.instapay_handle || '',
					wallet_provider: pm.wallet_provider || '',
					wallet_phone: pm.wallet_phone || '',
					card_brand: pm.card_brand || '',
					card_last4: pm.card_last4 || '',
					is_default: !!pm.is_default
				});
				this.isAddPaymentModalOpen = true;
			},

			/* Adding posts to the collection; editing posts to the method. */
			paymentFormAction() {
				if (this.paymentEditID) {
					return this.base + '/payment-methods/' + this.paymentEditID + '/edit';
				}
				return this.base + '/payment-methods';
			}
		};
	}

	window.dawaWallet = dawaWallet;
})();
