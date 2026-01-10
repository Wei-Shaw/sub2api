# Linux DO Connect

OAuth（Open Authorization）是一个开放的网络授权标准，目前最新版本为 OAuth 2.0。我们日常使用的第三方登录（如 Google 账号登录）就采用了该标准。OAuth 允许用户授权第三方应用访问存储在其他服务提供商（如 Google）上的信息，无需在不同平台上重复填写注册信息。用户授权后，平台可以直接访问用户的账户信息进行身份验证，而用户无需向第三方应用提供密码。

目前系统已实现完整的 OAuth2 授权码（code）方式鉴权，但界面等配套功能还在持续完善中。让我们一起打造一个更完善的共享方案。

## 基本介绍

这是一套标准的 OAuth2 鉴权系统，可以让开发者共享论坛的用户基本信息。

- 可获取字段：

| 参数              | 说明                            |
| ----------------- | ------------------------------- |
| `id`              | 用户唯一标识（不可变）          |
| `username`        | 论坛用户名                      |
| `name`            | 论坛用户昵称（可变）            |
| `avatar_template` | 用户头像模板URL（支持多种尺寸） |
| `active`          | 账号活跃状态                    |
| `trust_level`     | 信任等级（0-4）                 |
| `silenced`        | 禁言状态                        |
| `external_ids`    | 外部ID关联信息                  |
| `api_key`         | API访问密钥                     |

通过这些信息，公益网站/接口可以实现：

1. 基于 `id` 的服务频率限制
2. 基于 `trust_level` 的服务额度分配
3. 基于用户信息的滥用举报机制

## 相关端点

- Authorize 端点： `https://connect.linux.do/oauth2/authorize`
- Token 端点：`https://connect.linux.do/oauth2/token`
- 用户信息 端点：`https://connect.linux.do/api/user`

## 申请使用

- 访问 [Connect.Linux.Do](https://connect.linux.do/) 申请接入你的应用。

![linuxdoconnect_1](https://wiki.linux.do/_next/image?url=%2Flinuxdoconnect_1.png&w=1080&q=75)

- 点击 **`我的应用接入`** - **`申请新接入`**，填写相关信息。其中 **`回调地址`** 是你的应用接收用户信息的地址。

![linuxdoconnect_2](https://wiki.linux.do/_next/image?url=%2Flinuxdoconnect_2.png&w=1080&q=75)

- 申请成功后，你将获得 **`Client Id`** 和 **`Client Secret`**，这是你应用的唯一身份凭证。

![linuxdoconnect_3](https://wiki.linux.do/_next/image?url=%2Flinuxdoconnect_3.png&w=1080&q=75)

## 接入 Linux Do

JavaScript
```JavaScript
// 安装第三方请求库（或使用原生的 Fetch API），本例中使用 axios
// npm install axios

// 通过 OAuth2 获取 Linux Do 用户信息的参考流程
const axios = require('axios');
const readline = require('readline');

// 配置信息（建议通过环境变量配置，避免使用硬编码）
const CLIENT_ID = '你的 Client ID';
const CLIENT_SECRET = '你的 Client Secret';
const REDIRECT_URI = '你的回调地址';
const AUTH_URL = 'https://connect.linux.do/oauth2/authorize';
const TOKEN_URL = 'https://connect.linux.do/oauth2/token';
const USER_INFO_URL = 'https://connect.linux.do/api/user';

// 第一步：生成授权 URL
function getAuthUrl() {
  const params = new URLSearchParams({
    client_id: CLIENT_ID,
    redirect_uri: REDIRECT_URI,
    response_type: 'code',
    scope: 'user'
  REDACTED);

  return `${AUTH_URLREDACTED?${params.toString()REDACTED`;
REDACTED

// 第二步：获取 code 参数
function getCode() {
  return new Promise((resolve) => {
    // 本例中使用终端输入来模拟流程，仅供本地测试
    // 请在实际应用中替换为真实的处理逻辑
    const rl = readline.createInterface({ input: process.stdin, output: process.stdout REDACTED);
    rl.question('从回调 URL 中提取出 code，粘贴到此处并按回车：', (answer) => {
      rl.close();
      resolve(answer.trim());
    REDACTED);
  REDACTED);
REDACTED

// 第三步：使用 code 参数获取访问令牌
async function getAccessToken(code) {
  try {
    const form = new URLSearchParams({
      client_id: CLIENT_ID,
      client_secret: CLIENT_SECRET,
      code: code,
      redirect_uri: REDIRECT_URI,
      grant_type: 'authorization_code'
    REDACTED).toString();

    const response = await axios.post(TOKEN_URL, form, {
      // 提醒：需正确配置请求头，否则无法正常获取访问令牌
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Accept': 'application/json'
      REDACTED
    REDACTED);

    return response.data;
  REDACTED catch (error) {
    console.error(`获取访问令牌失败：${error.response ? JSON.stringify(error.response.data) : error.messageREDACTED`);
    throw error;
  REDACTED
REDACTED

// 第四步：使用访问令牌获取用户信息
async function getUserInfo(accessToken) {
  try {
    const response = await axios.get(USER_INFO_URL, {
      headers: {
        Authorization: `Bearer ${accessTokenREDACTED`
      REDACTED
    REDACTED);

    return response.data;
  REDACTED catch (error) {
    console.error(`获取用户信息失败：${error.response ? JSON.stringify(error.response.data) : error.messageREDACTED`);
    throw error;
  REDACTED
REDACTED

// 主流程
async function main() {
  // 1. 生成授权 URL，前端引导用户访问授权页
  const authUrl = getAuthUrl();
  console.log(`请访问此 URL 授权：${authUrlREDACTED
`);

  // 2. 用户授权后，从回调 URL 获取 code 参数
  const code = await getCode();

  try {
    // 3. 使用 code 参数获取访问令牌
    const tokenData = await getAccessToken(code);
    const accessToken = tokenData.access_token;

    // 4. 使用访问令牌获取用户信息
    if (accessToken) {
      const userInfo = await getUserInfo(accessToken);
      console.log(`
获取用户信息成功：${JSON.stringify(userInfo, null, 2)REDACTED`);
    REDACTED else {
      console.log(`
获取访问令牌失败：${JSON.stringify(tokenData)REDACTED`);
    REDACTED
  REDACTED catch (error) {
    console.error('发生错误：', error);
  REDACTED
REDACTED
```
Python
```python
# 安装第三方请求库，本例中使用 requests
# pip install requests

# 通过 OAuth2 获取 Linux Do 用户信息的参考流程
import requests
import json

# 配置信息（建议通过环境变量配置，避免使用硬编码）
CLIENT_ID = '你的 Client ID'
CLIENT_SECRET = '你的 Client Secret'
REDIRECT_URI = '你的回调地址'
AUTH_URL = 'https://connect.linux.do/oauth2/authorize'
TOKEN_URL = 'https://connect.linux.do/oauth2/token'
USER_INFO_URL = 'https://connect.linux.do/api/user'

# 第一步：生成授权 URL
def get_auth_url():
    params = {
        'client_id': CLIENT_ID,
        'redirect_uri': REDIRECT_URI,
        'response_type': 'code',
        'scope': 'user'
    REDACTED
    auth_url = f"{AUTH_URLREDACTED?{'&'.join(f'{kREDACTED={vREDACTED' for k, v in params.items())REDACTED"
    return auth_url

# 第二步：获取 code 参数
def get_code():
    # 本例中使用终端输入来模拟流程，仅供本地测试
    # 请在实际应用中替换为真实的处理逻辑
    return input('从回调 URL 中提取出 code，粘贴到此处并按回车：').strip()

# 第三步：使用 code 参数获取访问令牌
def get_access_token(code):
    try:
        data = {
            'client_id': CLIENT_ID,
            'client_secret': CLIENT_SECRET,
            'code': code,
            'redirect_uri': REDIRECT_URI,
            'grant_type': 'authorization_code'
        REDACTED
        # 提醒：需正确配置请求头，否则无法正常获取访问令牌
        headers = {
            'Content-Type': 'application/x-www-form-urlencoded',
            'Accept': 'application/json'
        REDACTED
        response = requests.post(TOKEN_URL, data=data, headers=headers)
        response.raise_for_status()
        return response.json()
    except requests.exceptions.RequestException as e:
        print(f"获取访问令牌失败：{eREDACTED")
        return None

# 第四步：使用访问令牌获取用户信息
def get_user_info(access_token):
    try:
        headers = {
            'Authorization': f'Bearer {access_tokenREDACTED'
        REDACTED
        response = requests.get(USER_INFO_URL, headers=headers)
        response.raise_for_status()
        return response.json()
    except requests.exceptions.RequestException as e:
        print(f"获取用户信息失败：{eREDACTED")
        return None

# 主流程
if __name__ == '__main__':
    # 1. 生成授权 URL，前端引导用户访问授权页
    auth_url = get_auth_url()
    print(f'请访问此 URL 授权：{auth_urlREDACTED
')

    # 2. 用户授权后，从回调 URL 获取 code 参数
    code = get_code()

    # 3. 使用 code 参数获取访问令牌
    token_data = get_access_token(code)
    if token_data:
        access_token = token_data.get('access_token')

        # 4. 使用访问令牌获取用户信息
        if access_token:
            user_info = get_user_info(access_token)
            if user_info:
                print(f"
获取用户信息成功：{json.dumps(user_info, indent=2)REDACTED")
            else:
                print("
获取用户信息失败")
        else:
            print(f"
获取访问令牌失败：{json.dumps(token_data, indent=2)REDACTED")
    else:
        print("
获取访问令牌失败")
```
PHP
```php
// 通过 OAuth2 获取 Linux Do 用户信息的参考流程

// 配置信息
$CLIENT_ID = '你的 Client ID';
$CLIENT_SECRET = '你的 Client Secret';
$REDIRECT_URI = '你的回调地址';
$AUTH_URL = 'https://connect.linux.do/oauth2/authorize';
$TOKEN_URL = 'https://connect.linux.do/oauth2/token';
$USER_INFO_URL = 'https://connect.linux.do/api/user';

// 生成授权 URL
function getAuthUrl($clientId, $redirectUri) {
    global $AUTH_URL;
    return $AUTH_URL . '?' . http_build_query([
        'client_id' => $clientId,
        'redirect_uri' => $redirectUri,
        'response_type' => 'code',
        'scope' => 'user'
    ]);
REDACTED

// 使用 code 参数获取用户信息（合并获取令牌和获取用户信息的步骤）
function getUserInfoWithCode($code, $clientId, $clientSecret, $redirectUri) {
    global $TOKEN_URL, $USER_INFO_URL;

    // 1. 获取访问令牌
    $ch = curl_init($TOKEN_URL);
    curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
    curl_setopt($ch, CURLOPT_POST, true);
    curl_setopt($ch, CURLOPT_POSTFIELDS, http_build_query([
        'client_id' => $clientId,
        'client_secret' => $clientSecret,
        'code' => $code,
        'redirect_uri' => $redirectUri,
        'grant_type' => 'authorization_code'
    ]));
    curl_setopt($ch, CURLOPT_HTTPHEADER, [
        'Content-Type: application/x-www-form-urlencoded',
        'Accept: application/json'
    ]);

    $tokenResponse = curl_exec($ch);
    curl_close($ch);

    $tokenData = json_decode($tokenResponse, true);
    if (!isset($tokenData['access_token'])) {
        return ['error' => '获取访问令牌失败', 'details' => $tokenData];
    REDACTED

    // 2. 获取用户信息
    $ch = curl_init($USER_INFO_URL);
    curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
    curl_setopt($ch, CURLOPT_HTTPHEADER, [
        'Authorization: Bearer ' . $tokenData['access_token']
    ]);

    $userResponse = curl_exec($ch);
    curl_close($ch);

    return json_decode($userResponse, true);
REDACTED

// 主流程
// 1. 生成授权 URL
$authUrl = getAuthUrl($CLIENT_ID, $REDIRECT_URI);
echo "<a href='$authUrl'>使用 Linux Do 登录</a>";

// 2. 处理回调并获取用户信息
if (isset($_GET['code'])) {
    $userInfo = getUserInfoWithCode(
        $_GET['code'],
        $CLIENT_ID,
        $CLIENT_SECRET,
        $REDIRECT_URI
    );

    if (isset($userInfo['error'])) {
        echo '错误: ' . $userInfo['error'];
    REDACTED else {
        echo '欢迎, ' . $userInfo['name'] . '!';
        // 处理用户登录逻辑...
    REDACTED
REDACTED
```

## 使用说明

### 授权流程

1. 用户点击应用中的’使用 Linux Do 登录’按钮
2. 系统将用户重定向至 Linux Do 的授权页面
3. 用户完成授权后，系统自动重定向回应用并携带授权码
4. 应用使用授权码获取访问令牌
5. 使用访问令牌获取用户信息

### 安全建议

- 切勿在前端代码中暴露 Client Secret
- 对所有用户输入数据进行严格验证
- 确保使用 HTTPS 协议传输数据
- 定期更新并妥善保管 Client Secret