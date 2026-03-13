// API Client — fetch wrapper for ReMS backend
// All responses follow: { success: boolean, status: number, message: string, data: T }

const BASE_URL = '/api/v1';

interface ApiResponse<T = unknown> {
	success: boolean;
	status: number;
	message: string;
	data: T;
}

function getToken(): string | null {
	if (typeof window === 'undefined') return null;
	return localStorage.getItem('rems_token');
}

function getRefreshToken(): string | null {
	if (typeof window === 'undefined') return null;
	return localStorage.getItem('rems_refresh_token');
}

function setTokens(token: string, refreshToken: string) {
	localStorage.setItem('rems_token', token);
	localStorage.setItem('rems_refresh_token', refreshToken);
}

function clearTokens() {
	localStorage.removeItem('rems_token');
	localStorage.removeItem('rems_refresh_token');
	localStorage.removeItem('rems_user');
}

async function refreshAccessToken(): Promise<boolean> {
	const refreshToken = getRefreshToken();
	if (!refreshToken) return false;

	try {
		const res = await fetch(`${BASE_URL}/auth/refresh`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ refresh_token: refreshToken })
		});

		const json: ApiResponse<{ token: string; refresh_token: string }> = await res.json();

		if (json.success && json.data) {
			setTokens(json.data.token, json.data.refresh_token);
			return true;
		}
	} catch {
		// refresh failed
	}

	clearTokens();
	return false;
}

async function request<T = unknown>(
	method: string,
	path: string,
	body?: unknown,
	retry = true
): Promise<ApiResponse<T>> {
	const headers: Record<string, string> = {
		'Content-Type': 'application/json'
	};

	const token = getToken();
	if (token) {
		headers['Authorization'] = `Bearer ${token}`;
	}

	const opts: RequestInit = { method, headers };
	if (body && method !== 'GET') {
		opts.body = JSON.stringify(body);
	}

	const res = await fetch(`${BASE_URL}${path}`, opts);
	const json: ApiResponse<T> = await res.json();

	// Auto-refresh on 401
	if (res.status === 401 && retry) {
		const refreshed = await refreshAccessToken();
		if (refreshed) {
			return request<T>(method, path, body, false);
		}
		// Redirect to login
		if (typeof window !== 'undefined') {
			clearTokens();
			window.location.href = '/login';
		}
	}

	return json;
}

export const api = {
	get: <T = unknown>(path: string) => request<T>('GET', path),
	post: <T = unknown>(path: string, body?: unknown) => request<T>('POST', path, body),
	put: <T = unknown>(path: string, body?: unknown) => request<T>('PUT', path, body),
	del: <T = unknown>(path: string) => request<T>('DELETE', path),
	setTokens,
	clearTokens,
	getToken
};
