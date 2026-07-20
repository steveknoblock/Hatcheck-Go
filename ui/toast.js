function showToast(label, msg, isError = false, onClick = null) {
    const toast = document.getElementById('toast');
    document.getElementById('toast-label').textContent = label;
    document.getElementById('toast-msg').textContent = msg;
    toast.classList.toggle('error', isError);
    toast.classList.toggle('clickable', !!onClick);
    toast.onclick = onClick || null;
    toast.classList.add('show');
    setTimeout(() => toast.classList.remove('show'), onClick ? 5000 : 3000);
}