const params = new URLSearchParams(location.search);
const theme = params.get('theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
document.documentElement.dataset.theme = theme;

const CONTACT_CONFIG = {
  qrImageUrl: 'https://pub-2a15421b492148ac9cbbe9b46536f805.r2.dev/1dbd0d8fe575fa4ff7040c6480d0ab67%20(2).jpg',
  groupNumber: '1026260003',
  supportEmail: 'cozy.amain@foxmail.com',
  serviceHours: '09:00 - 23:00'
};

const qrBox = document.getElementById('qrBox');
const groupNumber = document.getElementById('groupNumber');
const supportEmail = document.getElementById('supportEmail');
const serviceHours = document.getElementById('serviceHours');
const copyGroupBtn = document.getElementById('copyGroupBtn');
const mailBtn = document.getElementById('mailBtn');

if (CONTACT_CONFIG.qrImageUrl) {
  qrBox.innerHTML = '';
  const img = document.createElement('img');
  img.src = CONTACT_CONFIG.qrImageUrl;
  img.alt = '客服二维码';
  qrBox.appendChild(img);
}

groupNumber.textContent = CONTACT_CONFIG.groupNumber || '待填写';
supportEmail.textContent = CONTACT_CONFIG.supportEmail || '待填写';
serviceHours.textContent = CONTACT_CONFIG.serviceHours || '09:00 - 23:00';

if (CONTACT_CONFIG.supportEmail) {
  mailBtn.href = `mailto:${CONTACT_CONFIG.supportEmail}`;
} else {
  mailBtn.addEventListener('click', event => event.preventDefault());
}

copyGroupBtn.addEventListener('click', async () => {
  if (!CONTACT_CONFIG.groupNumber) return;
  await navigator.clipboard.writeText(CONTACT_CONFIG.groupNumber);
  copyGroupBtn.textContent = '已复制';
  setTimeout(() => { copyGroupBtn.textContent = '复制群号'; }, 1600);
});
