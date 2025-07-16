import { createClient, ConnectError, Code } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { Modal } from 'bootstrap';
import { APIService } from '../generated/internalapi/v1/api_pb';
import { setServerAddr, getServerAddr, joinUrl } from './helpers';

let loginModal: Modal | null = null;

document.addEventListener('DOMContentLoaded', () => {
  const form = document.getElementById('serverForm') as HTMLFormElement;
  form.addEventListener('submit', handleServerFormSubmit);
  const serverAddrInput = form.elements.namedItem(
    'serverAddress',
  ) as HTMLInputElement;
  serverAddrInput.value = getServerAddr() ?? window.location.origin;

  const loginForm = document.getElementById('loginForm') as HTMLFormElement;
  loginForm.addEventListener('submit', handleLoginFormSubmit);

  const passwordInput = document.getElementById(
    'adminPassword',
  ) as HTMLInputElement;
  passwordInput.addEventListener('input', () => {
    clearLoginErrors();
  });
});

async function handleServerFormSubmit(event: Event) {
  event.preventDefault();
  console.log('Server form submitted');

  const form = event.target as HTMLFormElement;
  const formData = new FormData(form);

  const serverAddr = formData.get('serverAddress') as string;
  const transport = createConnectTransport({ baseUrl: serverAddr });
  const client = createClient(APIService, transport);

  try {
    await client.authenticate({}, { timeoutMs: 5000 });
    setServerAddr(serverAddr);
    window.location.href = '/';
  } catch (error) {
    // TODO: modal
    if (!(error instanceof ConnectError)) {
      alert('Unexpected error occurred: ' + error);
      return;
    }

    switch (error.code) {
      case Code.Unauthenticated:
        setServerAddr(serverAddr);
        console.info('Login required, showing login modal...');
        showLoginModal();
        break;
      case Code.DeadlineExceeded:
        // TODO: modal
        alert('Request timed out. Please check your connection and try again.');
        console.error('Request timed out:', error);
        break;
      default:
        // TODO: modal
        alert('An error occurred: ' + error.message);
        console.error('An error occurred:', error);
        break;
    }
  }
}

function showLoginModal() {
  const loginModalElement = document.getElementById('login-modal');
  const serverFormContainer = document.getElementById('serverFormContainer');

  if (!loginModalElement || !serverFormContainer) {
    return;
  }

  loginModal = new Modal(loginModalElement);

  // Handle modal events
  loginModalElement.addEventListener('shown.bs.modal', () => {
    // Additional dimming for the server form
    serverFormContainer.classList.add('dimmed');

    // Focus on password field
    const passwordInput = document.getElementById('adminPassword');
    passwordInput?.focus();
  });

  loginModalElement.addEventListener('hidden.bs.modal', () => {
    // Restore the server form when modal closes
    serverFormContainer.classList.remove('dimmed');
  });

  loginModal.show();
}

async function handleLoginFormSubmit(event: Event) {
  event.preventDefault();

  const form = event.target as HTMLFormElement;
  const formData = new FormData(form);
  const password = formData.get('password') as string;
  const serverAddr = getServerAddr();

  if (!serverAddr) {
    alert('No server address configured');
    return;
  }

  // Basic validation
  if (!password || password.trim() === '') {
    showLoginError('Please enter the admin password.', 'password');
    return;
  }

  // Clear any previous errors
  clearLoginErrors();

  // Show loading state
  setLoginLoadingState(true);

  try {
    const response = await fetch(joinUrl(serverAddr, 'session'), {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/json',
      },
      body: JSON.stringify({ password }),
    });

    const result = await response.json();

    if (result.success) {
      // Success - redirect to dashboard
      window.location.href = result.redirect || '/';
    } else {
      // Error - show validation feedback
      showLoginError(result.error || 'Login failed', result.field);
    }
  } catch (error) {
    console.error('Login error:', error);
    showLoginError(
      'Unable to connect to server. Please check your connection and try again.',
    );
  } finally {
    setLoginLoadingState(false);
  }
}

function clearLoginErrors() {
  const passwordInput = document.getElementById(
    'adminPassword',
  ) as HTMLInputElement;
  const errorDiv = document.getElementById('password-error');

  if (passwordInput) {
    passwordInput.classList.remove('is-invalid');
  }

  if (errorDiv) {
    errorDiv.textContent = 'Please enter the admin password.';
  }
}

function showLoginError(message: string, _field?: string) {
  const passwordInput = document.getElementById(
    'adminPassword',
  ) as HTMLInputElement;
  const errorDiv = document.getElementById('password-error');

  if (passwordInput) {
    passwordInput.classList.add('is-invalid');
    passwordInput.focus();
    passwordInput.select();
  }

  if (errorDiv) {
    errorDiv.textContent = message;
  }
}

function setLoginLoadingState(loading: boolean) {
  const submitButton = document.querySelector(
    '#loginForm button[type="submit"]',
  ) as HTMLButtonElement;
  const submitText = document.getElementById('login-submit-text');
  const submitSpinner = document.getElementById('login-submit-spinner');

  if (submitButton) {
    submitButton.disabled = loading;
  }

  if (submitText) {
    submitText.textContent = loading ? 'Logging in...' : 'Log in';
  }

  if (submitSpinner) {
    submitSpinner.style.display = loading ? 'inline-block' : 'none';
  }
}
