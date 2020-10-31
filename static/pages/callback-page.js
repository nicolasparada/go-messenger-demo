import { navigate } from 'https://unpkg.com/@nicolasparada/router@0.8.0/router.js';
import http from '../http.js';

export default async function callbackPage() {
    const url = new URL(location.toString())
    let token = url.searchParams.get('token')
    let expiresAt = url.searchParams.get('expires_at')

    try {
        if (token === null || expiresAt === null) {
            throw new Error('Invalid URL')
        }

        token = decodeURIComponent(token)
        expiresAt = decodeURIComponent(expiresAt)

        const authUser = await getAuthUser(token)

        localStorage.setItem('auth_user', JSON.stringify(authUser))
        localStorage.setItem('token', token)
        localStorage.setItem('expires_at', expiresAt)
    } catch (err) {
        alert(err.message)
    } finally {
        navigate('/', true)
    }
}

function getAuthUser(token) {
    return http.get('/api/auth_user', { authorization: `Bearer ${token}` })
}
