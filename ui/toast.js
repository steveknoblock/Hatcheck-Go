function showToast(label, msg, isError = false) {
    const toast = document.getElementById('toast');
    document.getElementById('toast-label').textContent = label;
    document.getElementById('toast-msg').textContent = msg;
    toast.classList.toggle('error', isError);
    toast.classList.add('show');
    setTimeout(() => toast.classList.remove('show'), 3000);
}