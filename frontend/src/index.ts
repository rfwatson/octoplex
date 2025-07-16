import { ConnectError } from '@connectrpc/connect';
import { Modal } from 'bootstrap';
import { getServerAddr } from './helpers';
import type { AppState, AppCommand, AppEvent } from './types.ts';
import { WebSocketCommunicator } from './websocket-communicator.ts';

class Dashboard {
  private communicator: WebSocketCommunicator | null = null;
  private state: AppState = {
    source: {
      live: false,
      liveChangedAt: null,
      tracks: [],
      rtmpURL: '',
      rtmpsURL: '',
      container: {
        status: '',
        healthState: '',
        cpuPercent: 0,
        memoryUsageBytes: 0,
        rxRate: 0,
        txRate: 0,
        pullPercent: 0,
        pullStatus: '',
        pullProgress: '',
        imageName: '',
        err: null,
      },
    },
    destinations: [],
  };
  private isConnected = false;

  async init() {
    let serverAddr = getServerAddr();
    if (!serverAddr) {
      console.log('No server address configured.');
      window.location.href = '/login.html';
      return;
    }

    // TODO: connect directly with connectrpc if H2 is available.
    try {
      this.communicator = new WebSocketCommunicator({
        baseUrl: serverAddr,
        onEvent: (event) => this.handleEvent(event),
        onError: (error) => this.handleError(error),
        onUnauthorized: () => this.handleUnauthorized(),
        onConnectionStateChange: (connected) =>
          this.handleConnectionStateChange(connected),
      });

      // Connect to the backend via WebSocket
      await this.communicator.connect();

      // Setup UI event listeners
      this.setupEventListeners();

      console.log('Dashboard initialized successfully');
    } catch (error) {
      if (error instanceof ConnectError) {
        alert('Authentication failed: ' + error.message);
      } else {
        alert('Unexpected error: ' + error);
      }

      console.error('Dashboard initialization failed:', error);
    }
  }

  private handleError(error: Error) {
    console.error('Communication error:', error);
    this.showToast(`Connection error: ${error.message}`, 'error');
  }

  private handleUnauthorized() {
    console.error('Unauthorized access - redirecting to login');
    window.location.href = '/login.html';
  }

  private handleEvent(event: AppEvent) {
    switch (event.type) {
      case 'appStateChanged':
        this.state = event.state;
        this.updateUI();
        break;

      case 'handshakeCompleted':
        console.log('Handshake completed - ready to receive app state');
        break;

      case 'addDestinationFailed':
      case 'updateDestinationFailed':
        console.error('Destination operation failed:', event);
        this.handleSaveDestinationError(event.error);
        break;

      case 'destinationAdded':
        this.showToast('Destination added successfully', 'success');
        this.closeDestinationModal();
        break;
      case 'destinationUpdated':
        this.showToast('Destination updated successfully', 'success');
        this.closeDestinationModal();
        break;
      case 'destinationRemoved':
        this.showToast('Destination removed successfully', 'success');
        break;
      case 'destinationStarted':
        this.showToast('Destination started', 'success');
        break;
      case 'destinationStopped':
        this.showToast('Destination stopped', 'success');
        break;
      case 'destinationStreamExited':
        console.log(`Destination stream exited:`, event);
        this.showToast(
          `Stream to ${event.name} exited: ${event.error}`,
          'error',
        );
        break;

      case 'error': // fatal error
        console.error('Backend error:', event.message);
        this.showToast(event.message, 'error');
        break;

      default:
        console.log('Unhandled event type:', event);
    }
  }

  private handleConnectionStateChange(connected: boolean) {
    this.isConnected = connected;
    this.updateConnectionStatus();

    if (connected) {
      console.log('Connected to backend');
      this.showToast('Connected to backend', 'success');
    } else {
      console.log('Disconnected from backend');
      this.showToast('Disconnected from backend', 'error');
    }
  }

  private updateConnectionStatus() {
    const statusIndicator = document.getElementById('connection-status');
    if (statusIndicator) {
      statusIndicator.className = this.isConnected
        ? 'badge bg-success'
        : 'badge bg-danger';
      statusIndicator.innerHTML = this.isConnected
        ? '<i class="bi bi-wifi me-1"></i>Connected'
        : '<i class="bi bi-wifi-off me-1"></i>Disconnected';
    }
  }

  private updateUI() {
    this.updateSourcePanel();
    this.updateDestinationsTable();
    this.updateSourceURLs();
  }

  private updateSourcePanel() {
    const { source } = this.state;

    // Update source status
    const statusEl = document.getElementById('source-status');
    if (statusEl) {
      if (source.live) {
        statusEl.innerHTML =
          '<span class="badge bg-success fs-6"><i class="bi bi-broadcast me-1"></i>Receiving</span>';
      } else if (
        source.container.status === 'running' &&
        source.container.healthState === 'healthy'
      ) {
        statusEl.innerHTML =
          '<span class="badge bg-warning fs-6"><i class="bi bi-clock me-1"></i>Waiting</span>';
      } else {
        statusEl.innerHTML =
          '<span class="badge bg-danger fs-6"><i class="bi bi-exclamation-triangle me-1"></i>Not ready</span>';
      }
    }

    // Update tracks
    const tracksEl = document.getElementById('source-tracks');
    if (tracksEl) {
      tracksEl.textContent =
        source.tracks.length > 0 ? source.tracks.join(', ') : '—';
    }

    // Update health
    const healthEl = document.getElementById('source-health');
    if (healthEl) {
      healthEl.textContent = source.container.healthState || '—';
    }

    // Update CPU
    const cpuEl = document.getElementById('source-cpu');
    if (cpuEl) {
      cpuEl.textContent =
        source.container.status === 'running'
          ? Number(source.container.cpuPercent).toFixed(1)
          : '—';
    }

    // Update Memory
    const memEl = document.getElementById('source-mem');
    if (memEl) {
      memEl.textContent =
        source.container.status === 'running'
          ? (Number(source.container.memoryUsageBytes) / 1024 / 1024).toFixed(1)
          : '—';
    }

    // Update Rx Rate
    const rxEl = document.getElementById('source-rx');
    if (rxEl) {
      rxEl.textContent =
        source.container.status === 'running'
          ? Number(source.container.rxRate).toString()
          : '—';
    }
  }

  private updateDestinationsTable() {
    const tbody = document.getElementById('destinations-tbody');
    if (!tbody) return;

    const noDestRow = document.getElementById('no-destinations-row');

    // Clear existing rows except the "no destinations" row
    const existingRows = tbody.querySelectorAll('tr:not(#no-destinations-row)');
    existingRows.forEach((row) => row.remove());

    if (this.state.destinations.length === 0) {
      if (noDestRow) noDestRow.style.display = '';
      return;
    }

    if (noDestRow) noDestRow.style.display = 'none';

    // Add destination rows
    this.state.destinations.forEach((dest) => {
      const row = document.createElement('tr');

      const isLive = dest.status === 'live';
      const canStart = this.state.source.live && !isLive;

      row.innerHTML = `
        <td>${this.escapeHtml(dest.name)}</td>
        <td>
          ${
            isLive
              ? '<span class="badge bg-success"><i class="bi bi-play-fill me-1"></i>Sending</span>'
              : dest.status === 'starting'
                ? '<span class="badge bg-warning"><i class="bi bi-arrow-clockwise me-1"></i>Starting</span>'
                : '<span class="badge bg-secondary"><i class="bi bi-stop-fill me-1"></i>Off-air</span>'
          }
        </td>
        <td class="d-none d-md-table-cell">${dest.container.status || '—'}</td>
        <td class="d-none d-lg-table-cell">${dest.status === 'live' ? 'healthy' : '—'}</td>
        <td class="d-none d-lg-table-cell">${dest.container.status === 'running' ? Number(dest.container.cpuPercent).toFixed(1) : '—'}</td>
        <td class="d-none d-lg-table-cell">${dest.container.status === 'running' ? (Number(dest.container.memoryUsageBytes) / 1000 / 1000).toFixed(1) : '—'}</td>
        <td class="d-none d-xl-table-cell">${dest.container.status === 'running' ? Number(dest.container.txRate).toString() : '—'}</td>
        <td>
          <div class="destination-actions">
            ${
              isLive
                ? `<button type="button" class="btn btn-danger" onclick="dashboard.stopDestination('${dest.id}')">
                   <i class="bi bi-stop-fill me-1"></i>Stop
                 </button>`
                : canStart
                  ? `<button type="button" class="btn btn-success" onclick="dashboard.startDestination('${dest.id}')">
                     <i class="bi bi-play-fill me-1"></i>Start
                   </button>`
                  : `<button type="button" class="btn btn-secondary" disabled>
                     <i class="bi bi-play-fill me-1"></i>Start
                   </button>`
            }
            <button type="button" class="btn btn-secondary d-none d-sm-inline-block" onclick="dashboard.editDestination('${dest.id}')" ${isLive ? 'disabled' : ''}>
              <i class="bi bi-pencil me-1"></i>Edit
            </button>
            <button type="button" class="btn btn-remove" onclick="dashboard.removeDestination('${dest.id}')" title="Remove destination">
              <i class="bi bi-trash"></i>
            </button>
          </div>
        </td>
      `;
      tbody.appendChild(row);
    });
  }

  private updateSourceURLs() {
    const { source } = this.state;

    const rtmpSection = document.getElementById('rtmp-url-section');
    const rtmpsSection = document.getElementById('rtmps-url-section');
    const noUrlsMessage = document.getElementById('no-urls-message');
    const rtmpInput = document.getElementById(
      'rtmp-url-input',
    ) as HTMLInputElement;
    const rtmpsInput = document.getElementById(
      'rtmps-url-input',
    ) as HTMLInputElement;

    let hasAnyUrls = false;

    // Handle RTMP URL
    if (source.rtmpURL && rtmpSection && rtmpInput) {
      rtmpSection.style.display = 'block';
      rtmpInput.value = source.rtmpURL;
      hasAnyUrls = true;
    } else if (rtmpSection) {
      rtmpSection.style.display = 'none';
    }

    // Handle RTMPS URL
    if (source.rtmpsURL && rtmpsSection && rtmpsInput) {
      rtmpsSection.style.display = 'block';
      rtmpsInput.value = source.rtmpsURL;
      hasAnyUrls = true;
    } else if (rtmpsSection) {
      rtmpsSection.style.display = 'none';
    }

    // Show/hide "no URLs" message
    if (noUrlsMessage) {
      noUrlsMessage.style.display = hasAnyUrls ? 'none' : 'block';
    }
  }

  private escapeHtml(text: string): string {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  private setupEventListeners() {
    // Add destination button
    const addBtn = document.getElementById('add-destination-btn');
    addBtn?.addEventListener('click', () => this.addDestination());

    // Save destination button
    const saveBtn = document.getElementById('save-destination-btn');
    saveBtn?.addEventListener('click', () => this.saveDestination());

    // Copy URL buttons
    const copyRtmpBtn = document.getElementById('copy-rtmp-btn');
    copyRtmpBtn?.addEventListener('click', () => this.copyToClipboard('rtmp'));

    const copyRtmpsBtn = document.getElementById('copy-rtmps-btn');
    copyRtmpsBtn?.addEventListener('click', () =>
      this.copyToClipboard('rtmps'),
    );

    // Logout button
    const logoutLink = document.getElementById('logout-link');
    logoutLink?.addEventListener('click', (e) => {
      e.preventDefault();
      this.showLogoutConfirmation();
    });

    // Logout confirmation button
    const logoutConfirmBtn = document.getElementById('logout-confirm-btn');
    logoutConfirmBtn?.addEventListener('click', () => this.performLogout());
  }

  private async copyToClipboard(urlType: 'rtmp' | 'rtmps') {
    const url =
      urlType === 'rtmp'
        ? this.state.source.rtmpURL
        : this.state.source.rtmpsURL;
    if (!url) return;

    try {
      await navigator.clipboard.writeText(url);
      this.showToast(
        `${urlType.toUpperCase()} URL copied to clipboard`,
        'success',
      );
    } catch (err) {
      console.error('Failed to copy to clipboard:', err);
      this.showToast('Failed to copy URL', 'error');
    }
  }

  private showToast(message: string, type: 'success' | 'error') {
    const toast = document.createElement('div');
    toast.className = `alert alert-${type === 'success' ? 'success' : 'danger'} position-fixed toast-notification`;

    // Calculate position based on existing toasts
    const existingToasts = document.querySelectorAll('.toast-notification');
    const topOffset = 80 + existingToasts.length * 70; // 70px spacing between toasts

    toast.style.top = `${topOffset}px`;
    toast.style.right = '20px';
    toast.style.zIndex = '9999';
    toast.style.maxWidth = '400px';
    toast.style.borderRadius = '8px';
    toast.style.boxShadow = '0 4px 12px rgba(0, 0, 0, 0.3)';
    toast.style.transition = 'all 0.3s ease-in-out';
    toast.textContent = message;

    document.body.appendChild(toast);

    // Auto-remove and reposition remaining toasts
    setTimeout(() => {
      toast.style.opacity = '0';
      toast.style.transform = 'translateX(100%)';
      setTimeout(() => {
        toast.remove();
        this.repositionToasts();
      }, 300);
    }, 3000);
  }

  private repositionToasts() {
    const toasts = document.querySelectorAll('.toast-notification');
    toasts.forEach((toast, index) => {
      const element = toast as HTMLElement;
      element.style.top = `${80 + index * 70}px`;
    });
  }

  private showLogoutConfirmation() {
    const logoutModal = document.getElementById('logout-modal');
    if (logoutModal) {
      const modal = new Modal(logoutModal);
      modal.show();
    }
  }

  private performLogout() {
    const baseUrl = getServerAddr();
    if (!baseUrl) {
      window.location.href = '/login.html';
      return;
    }

    // Create a form and submit it to logout
    const form = document.createElement('form');
    form.method = 'POST';
    form.action = new URL('session/destroy', baseUrl).href;
    document.body.appendChild(form);
    form.submit();
  }

  // Public methods called by onclick handlers
  addDestination() {
    this.openDestinationModal();
  }

  async startDestination(destinationId: string) {
    console.log('Start destination:', destinationId);
    await this.sendCommand({
      type: 'startDestination',
      id: destinationId,
    });
  }

  async stopDestination(destinationId: string) {
    console.log('Stop destination:', destinationId);
    await this.sendCommand({
      type: 'stopDestination',
      id: destinationId,
    });
  }

  editDestination(destinationId: string) {
    const destination = this.state.destinations.find(
      (d) => d.id === destinationId,
    );
    if (destination) {
      this.openDestinationModal(destination);
    }
  }

  async removeDestination(destinationId: string) {
    const destination = this.state.destinations.find(
      (d) => d.id === destinationId,
    );
    const isLive = destination?.status === 'live';

    const message = isLive
      ? 'This destination is currently live. Force remove?'
      : 'Are you sure you want to remove this destination?';

    const confirmed = await this.showConfirmModal(
      'Remove Destination',
      message,
    );
    if (!confirmed) return;

    await this.sendCommand({
      type: 'removeDestination',
      id: destinationId,
      force: isLive,
    });
  }

  private async sendCommand(command: AppCommand) {
    if (!this.communicator) {
      console.error('Communicator not initialized');
      this.showToast('Not connected to backend', 'error');
      return;
    }

    try {
      await this.communicator.sendCommand(command);
      console.log('Command sent:', command);
    } catch (error) {
      console.error('Failed to send command:', error);
      this.showToast('Failed to send command', 'error');
    }
  }

  cleanup() {
    if (this.communicator) {
      this.communicator.disconnect();
    }
  }

  private openDestinationModal(destination?: any) {
    const modal = document.getElementById('destination-modal');
    const modalTitle = document.getElementById('destination-modal-label');
    const form = document.getElementById('destination-form') as HTMLFormElement;

    if (!modal || !modalTitle || !form) return;

    // Set modal title
    modalTitle.textContent = destination
      ? 'Edit Destination'
      : 'Add Destination';

    // Clear validation classes
    form.classList.remove('was-validated');
    form.querySelectorAll('.form-control').forEach((control) => {
      control.classList.remove('is-invalid');
    });
    this.clearSaveDestinationError();

    // Populate form if editing
    if (destination) {
      (form.elements.namedItem('name') as HTMLInputElement).value =
        destination.name || '';
      (form.elements.namedItem('url') as HTMLInputElement).value =
        destination.url || '';
    } else {
      // Reset form for new destination
      form.reset();
    }

    // Store destination ID for editing
    form.dataset.destinationId = destination?.id || '';

    // Show modal
    const bootstrapModal = new Modal(modal);
    bootstrapModal.show();
  }

  // handleSaveDestinationError handles a server error when saving a destination.
  //
  // TODO: improve error handling consistency between client-side and
  // server-side errors.
  private handleSaveDestinationError(msg: string) {
    const form = document.getElementById('destination-form') as HTMLFormElement;

    if (!form) return;

    const errElement = form.querySelector(
      '#destination-error',
    ) as HTMLDivElement;
    errElement.style.display = 'block';

    (errElement.querySelector('p') as HTMLParagraphElement).textContent = msg;
  }

  private clearSaveDestinationError() {
    const form = document.getElementById('destination-form') as HTMLFormElement;

    if (!form) return;

    (form.querySelector('#destination-error') as HTMLDivElement).style.display =
      'none';
  }

  private validateForm(form: HTMLFormElement): boolean {
    const name = (
      form.elements.namedItem('name') as HTMLInputElement
    ).value.trim();
    const url = (
      form.elements.namedItem('url') as HTMLInputElement
    ).value.trim();

    let valid = true;

    // Validate name
    const nameInput = form.elements.namedItem('name') as HTMLInputElement;
    if (!name) {
      nameInput.classList.add('is-invalid');
      valid = false;
    } else {
      nameInput.classList.remove('is-invalid');
    }

    // Validate URL
    const urlInput = form.elements.namedItem('url') as HTMLInputElement;
    if (!url) {
      urlInput.classList.add('is-invalid');
      return false;
    }

    try {
      new URL(url);
      urlInput.classList.remove('is-invalid');
    } catch {
      urlInput.classList.add('is-invalid');
      return false;
    }

    return valid;
  }

  private async saveDestination() {
    const form = document.getElementById('destination-form') as HTMLFormElement;
    if (!form) return;

    this.clearSaveDestinationError();

    if (!this.validateForm(form)) {
      form.classList.add('was-validated');
      return;
    }

    const formData = new FormData(form);
    const destinationId = form.dataset.destinationId;

    const data = {
      name: formData.get('name') as string,
      url: formData.get('url') as string,
    };

    try {
      if (destinationId) {
        await this.sendCommand({
          type: 'updateDestination',
          id: destinationId,
          ...data,
        });
      } else {
        await this.sendCommand({
          type: 'addDestination',
          ...data,
        });
      }
    } catch (error) {
      this.showToast('Failed to save destination', 'error');
      console.error('Error saving destination:', error);
    }
  }

  private closeDestinationModal() {
    const modal = document.getElementById('destination-modal');
    if (modal) {
      const bootstrapModal = Modal.getInstance(modal);
      if (bootstrapModal) {
        bootstrapModal.hide();
      }
    }
  }

  private showConfirmModal(title: string, message: string): Promise<boolean> {
    return new Promise((resolve) => {
      const modal = document.getElementById('confirm-modal');
      const modalTitle = document.getElementById('confirm-modal-label');
      const modalMessage = document.getElementById('confirm-modal-message');
      const actionButton = document.getElementById('confirm-modal-action');

      if (!modal || !modalTitle || !modalMessage || !actionButton) {
        resolve(false);
        return;
      }

      // Set modal content
      modalTitle.textContent = title;
      modalMessage.textContent = message;

      // Remove existing event listeners
      const newActionButton = actionButton.cloneNode(true) as HTMLElement;
      actionButton.parentNode?.replaceChild(newActionButton, actionButton);

      // Add event listeners
      const handleConfirm = () => {
        bootstrapModal.hide();
        resolve(true);
      };

      const handleCancel = () => {
        bootstrapModal.hide();
        resolve(false);
      };

      newActionButton.addEventListener('click', handleConfirm);

      // Show modal
      const bootstrapModal = new Modal(modal);

      // Handle modal close events
      modal.addEventListener('hidden.bs.modal', handleCancel, { once: true });

      bootstrapModal.show();
    });
  }
}

// Initialize dashboard when DOM is ready
let dashboard: Dashboard | null = null;

document.addEventListener('DOMContentLoaded', async () => {
  dashboard = new Dashboard();
  await dashboard.init();

  // Expose dashboard globally for onclick handlers
  (window as any).dashboard = dashboard;
});

// Cleanup on page unload
window.addEventListener('beforeunload', () => {
  if (dashboard) {
    dashboard.cleanup();
  }
});
