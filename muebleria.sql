-- Mueblería "Casa Quindío" — esquema demo para la DBaaS.
-- Compatible con MariaDB y PostgreSQL: usa IDs explícitos para evitar la
-- divergencia AUTO_INCREMENT (MariaDB) vs SERIAL/IDENTITY (PostgreSQL).

CREATE TABLE categorias (
    id          INT PRIMARY KEY,
    nombre      VARCHAR(60) NOT NULL UNIQUE,
    descripcion VARCHAR(200)
);

CREATE TABLE proveedores (
    id          INT PRIMARY KEY,
    nombre      VARCHAR(120) NOT NULL,
    contacto    VARCHAR(80),
    telefono    VARCHAR(20),
    ciudad      VARCHAR(60)
);

CREATE TABLE productos (
    id              INT PRIMARY KEY,
    categoria_id    INT NOT NULL REFERENCES categorias(id),
    proveedor_id    INT REFERENCES proveedores(id),
    sku             VARCHAR(20) NOT NULL UNIQUE,
    nombre          VARCHAR(120) NOT NULL,
    descripcion     VARCHAR(300),
    material        VARCHAR(60),
    precio          DECIMAL(10,2) NOT NULL,
    stock           INT NOT NULL DEFAULT 0,
    creado_en       DATE NOT NULL
);

CREATE TABLE clientes (
    id          INT PRIMARY KEY,
    nombre      VARCHAR(120) NOT NULL,
    email       VARCHAR(120) UNIQUE,
    telefono    VARCHAR(20),
    ciudad      VARCHAR(60)
);

CREATE TABLE ventas (
    id          INT PRIMARY KEY,
    cliente_id  INT NOT NULL REFERENCES clientes(id),
    fecha       DATE NOT NULL,
    total       DECIMAL(12,2) NOT NULL,
    forma_pago  VARCHAR(20) NOT NULL,           -- efectivo | tarjeta | transferencia
    estado      VARCHAR(20) NOT NULL DEFAULT 'pagada'
);

CREATE TABLE ventas_detalle (
    venta_id        INT NOT NULL REFERENCES ventas(id),
    producto_id     INT NOT NULL REFERENCES productos(id),
    cantidad        INT NOT NULL,
    precio_unitario DECIMAL(10,2) NOT NULL,
    PRIMARY KEY (venta_id, producto_id)
);

-- Categorías
INSERT INTO categorias (id, nombre, descripcion) VALUES
    (1, 'Salas',      'Sofas, sillones y poltronas para sala'),
    (2, 'Comedores',  'Mesas, sillas y aparadores'),
    (3, 'Dormitorios','Camas, mesas de noche y closets'),
    (4, 'Oficina',    'Escritorios, sillas ergonomicas y bibliotecas'),
    (5, 'Exterior',   'Muebles para terraza y jardin');

-- Proveedores
INSERT INTO proveedores (id, nombre, contacto, telefono, ciudad) VALUES
    (1, 'Maderas del Quindio S.A.S', 'Hernan Botero',  '+57 6 7445500', 'Armenia'),
    (2, 'Forrajes y Tapices Cali',   'Luisa Mosquera', '+57 2 5567788', 'Cali'),
    (3, 'Industrias Acero Pereira',  'Andres Castano', '+57 6 3219090', 'Pereira'),
    (4, 'Importadora Asia Pacific',  'Mei Lin',        '+57 1 4488221', 'Bogota');

-- Productos
INSERT INTO productos (id, categoria_id, proveedor_id, sku, nombre, descripcion, material, precio, stock, creado_en) VALUES
    (101, 1, 2, 'SAL-3P-LIN',  'Sofa Lineal 3 Puestos',           'Sofa tapizado en lino con base de madera maciza', 'Madera + Lino',       2890000.00,  6, '2025-09-01'),
    (102, 1, 1, 'SAL-RKR-001', 'Mecedora Quindiana',              'Mecedora artesanal de guadua y cuero',            'Guadua + Cuero',       890000.00, 12, '2025-09-05'),
    (103, 2, 1, 'COM-MSA-6P',  'Mesa Comedor 6 Puestos',          'Mesa de comedor en cedro con acabado mate',       'Cedro',               1750000.00,  4, '2025-09-10'),
    (104, 2, 2, 'COM-SIL-LIN', 'Silla Comedor Lino',              'Silla tapizada en lino, juego de 6',              'Madera + Lino',        180000.00, 38, '2025-09-12'),
    (105, 3, 1, 'DOR-CAM-QSZ', 'Cama Queen Cedro',                'Cama Queen Size en cedro con base reforzada',     'Cedro',               2450000.00,  5, '2025-09-15'),
    (106, 3, 1, 'DOR-MSN-001', 'Mesa de Noche Clasica',           'Mesa de noche con dos cajones',                   'Cedro',                420000.00, 18, '2025-09-15'),
    (107, 4, 3, 'OFI-ESC-EXE', 'Escritorio Ejecutivo',            'Escritorio L con cajonera lateral',               'Aglomerado + Metal', 1320000.00,  7, '2025-10-01'),
    (108, 4, 4, 'OFI-SIL-ERG', 'Silla Ergonomica ProMesh',        'Silla de oficina con soporte lumbar y malla',     'Malla + Aluminio',     780000.00, 22, '2025-10-08'),
    (109, 5, 3, 'EXT-MSA-RTN', 'Mesa Terraza Ratan Sintetico',    'Mesa redonda 4 puestos para exterior',            'Aluminio + Ratan',     960000.00, 10, '2025-10-20'),
    (110, 5, 3, 'EXT-SIL-RTN', 'Silla Terraza Ratan Sintetico',   'Silla apilable resistente a la intemperie',       'Aluminio + Ratan',     145000.00, 40, '2025-10-20');

-- Clientes
INSERT INTO clientes (id, nombre, email, telefono, ciudad) VALUES
    (1, 'Andrea Lopez',     'andrea.lopez@correo.co',  '+57 300 555 0142', 'Armenia'),
    (2, 'Bruno Mejia',      'bruno.mejia@correo.co',   '+57 311 555 0098', 'Pereira'),
    (3, 'Carolina Trujillo','caro.trujillo@correo.co', '+57 320 555 0011', 'Manizales'),
    (4, 'Diego Patino',     'diego.patino@correo.co',  '+57 301 555 0467', 'Armenia'),
    (5, 'Elena Cardona',    'elena.cardona@correo.co', '+57 314 555 0820', 'Cali');

-- Ventas (cabecera)
INSERT INTO ventas (id, cliente_id, fecha, total, forma_pago, estado) VALUES
    (7001, 1, '2026-04-22', 3070000.00, 'tarjeta',       'pagada'),
    (7002, 2, '2026-04-28', 2470000.00, 'transferencia', 'pagada'),
    (7003, 3, '2026-05-01',  890000.00, 'efectivo',      'pagada'),
    (7004, 4, '2026-05-05', 1320000.00, 'tarjeta',       'pagada'),
    (7005, 5, '2026-05-09', 1395000.00, 'transferencia', 'pagada'),
    (7006, 1, '2026-05-13',  580000.00, 'tarjeta',       'pendiente');

-- Ventas (detalle)
INSERT INTO ventas_detalle (venta_id, producto_id, cantidad, precio_unitario) VALUES
    (7001, 101, 1, 2890000.00),  -- Sofa Lineal
    (7001, 104, 1,  180000.00),  -- + 1 silla comedor
    (7002, 103, 1, 1750000.00),  -- Mesa comedor
    (7002, 104, 4,  180000.00),  -- + 4 sillas
    (7003, 102, 1,  890000.00),  -- Mecedora
    (7004, 107, 1, 1320000.00),  -- Escritorio
    (7005, 109, 1,  960000.00),  -- Mesa terraza
    (7005, 110, 3,  145000.00),  -- + 3 sillas terraza
    (7006, 108, 1,  580000.00);  -- Silla ergonomica con descuento
