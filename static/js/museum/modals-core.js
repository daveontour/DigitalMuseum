'use strict';

// Base Modals object — sub-modules appended by subsequent script files
const Modals = {
    _openModal: (modalElement) => { modalElement.style.display = 'flex'; },
    _closeModal: (modalElement) => { modalElement.style.display = 'none'; },
};
// Inline onclick handlers resolve identifiers on window — expose Modals for AppConfig row buttons.
window.Modals = Modals;
